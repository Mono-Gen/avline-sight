// Package store はSQLite(WAL)による永続化（生データ・1分集約・イベント・スナップショット・設定）を担う。
package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// Store はDBハンドル。
type Store struct {
	DB       *sql.DB
	path     string
	readOnly bool
}

const schema = `
CREATE TABLE IF NOT EXISTS profiles (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 0,
  settings_json TEXT NOT NULL DEFAULT '{}', topology_layout_json TEXT NOT NULL DEFAULT '{}',
  watchlist_json TEXT NOT NULL DEFAULT '[]', background BLOB, background_type TEXT,
  created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS switches (
  id INTEGER PRIMARY KEY AUTOINCREMENT, profile_id INTEGER NOT NULL, name TEXT NOT NULL, group_tag TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL, snmp_port INTEGER NOT NULL DEFAULT 161, community_ro TEXT NOT NULL DEFAULT 'public',
  web_url TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', fw_version TEXT NOT NULL DEFAULT '',
  sys_object_id TEXT NOT NULL DEFAULT '', oid_capabilities_json TEXT NOT NULL DEFAULT '{}', enabled INTEGER NOT NULL DEFAULT 1);
CREATE TABLE IF NOT EXISTS ports (
  switch_id INTEGER NOT NULL, if_index INTEGER NOT NULL, if_name TEXT NOT NULL DEFAULT '',
  monitor_excluded INTEGER NOT NULL DEFAULT 0, expected_speed_mbps INTEGER NOT NULL DEFAULT 0,
  expected_av_role TEXT NOT NULL DEFAULT '', manual_memo TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (switch_id, if_index));
CREATE TABLE IF NOT EXISTS device_aliases (
  id INTEGER PRIMARY KEY, profile_id INTEGER NOT NULL, match_type TEXT NOT NULL, match_key TEXT NOT NULL,
  label TEXT NOT NULL, note TEXT NOT NULL DEFAULT '', UNIQUE(profile_id, match_type, match_key));
CREATE TABLE IF NOT EXISTS port_metrics_raw (
  ts INTEGER NOT NULL, switch_id INTEGER NOT NULL, if_index INTEGER NOT NULL, oper_up INTEGER, speed_mbps INTEGER,
  rx_bps REAL, tx_bps REAL, rx_mc_pps REAL, tx_mc_pps REAL, rx_bc_pps REAL,
  err_delta INTEGER, fcs_delta INTEGER, discard_delta INTEGER, cos_drop_delta INTEGER, pause_delta INTEGER,
  poe_mw REAL, sfp_rx_dbm REAL);
CREATE INDEX IF NOT EXISTS ix_pmr ON port_metrics_raw(switch_id, if_index, ts);
CREATE INDEX IF NOT EXISTS ix_pmr_ts ON port_metrics_raw(ts);
CREATE TABLE IF NOT EXISTS port_metrics_1m (
  ts_minute INTEGER NOT NULL, switch_id INTEGER NOT NULL, if_index INTEGER NOT NULL,
  rx_bps_avg REAL, rx_bps_max REAL, tx_bps_avg REAL, tx_bps_max REAL, rx_mc_pps_avg REAL, tx_mc_pps_avg REAL,
  err_sum INTEGER, fcs_sum INTEGER, discard_sum INTEGER, cos_drop_sum INTEGER, pause_sum INTEGER,
  poe_mw_avg REAL, sfp_rx_dbm_min REAL,
  PRIMARY KEY (switch_id, if_index, ts_minute));
CREATE TABLE IF NOT EXISTS switch_health_raw (
  ts INTEGER NOT NULL, switch_id INTEGER NOT NULL, cpu_5s REAL, cpu_60s REAL, cpu_300s REAL, mem_free_pct REAL,
  mbuf_used_pct REAL, mbuf_alloc_fails_delta INTEGER, temp_max_c REAL, poe_total_w REAL, sys_uptime_sec INTEGER);
CREATE INDEX IF NOT EXISTS ix_shr ON switch_health_raw(switch_id, ts);
CREATE TABLE IF NOT EXISTS switch_health_1m (
  ts_minute INTEGER NOT NULL, switch_id INTEGER NOT NULL, cpu_5s_avg REAL, cpu_5s_max REAL, mem_free_pct_min REAL,
  mbuf_used_pct_max REAL, temp_max_c REAL, poe_total_w_avg REAL, PRIMARY KEY (switch_id, ts_minute));
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY, ts INTEGER NOT NULL, switch_id INTEGER NOT NULL DEFAULT 0, switch_name TEXT NOT NULL DEFAULT '',
  if_index INTEGER NOT NULL DEFAULT 0, category TEXT NOT NULL, severity TEXT NOT NULL, code TEXT NOT NULL,
  title TEXT NOT NULL, detail_json TEXT NOT NULL DEFAULT '{}', hint_text TEXT NOT NULL DEFAULT '',
  acked INTEGER NOT NULL DEFAULT 0, acked_at INTEGER, dedupe_key TEXT);
CREATE INDEX IF NOT EXISTS ix_ev_ts ON events(ts);
CREATE UNIQUE INDEX IF NOT EXISTS ux_ev_dedupe ON events(switch_id, dedupe_key) WHERE dedupe_key IS NOT NULL;
CREATE TABLE IF NOT EXISTS state_snapshots (
  id INTEGER PRIMARY KEY, ts INTEGER NOT NULL, trigger TEXT NOT NULL, state_gz BLOB NOT NULL);
CREATE INDEX IF NOT EXISTS ix_ss_ts ON state_snapshots(ts);
CREATE TABLE IF NOT EXISTS device_bindings (
  id INTEGER PRIMARY KEY, switch_id INTEGER NOT NULL, if_index INTEGER NOT NULL, mac TEXT NOT NULL,
  oui_vendor TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL DEFAULT '', vlan_id INTEGER NOT NULL DEFAULT 0,
  lldp_sys_name TEXT NOT NULL DEFAULT '', first_seen_ts INTEGER NOT NULL, last_seen_ts INTEGER NOT NULL,
  is_active INTEGER NOT NULL DEFAULT 1);
CREATE INDEX IF NOT EXISTS ix_db ON device_bindings(switch_id, if_index, mac, is_active);
CREATE TABLE IF NOT EXISTS baselines (
  id INTEGER PRIMARY KEY, profile_id INTEGER NOT NULL, name TEXT NOT NULL, captured_ts INTEGER NOT NULL, snapshot_gz BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
`

// Open はDBを開き、スキーマを適用する。readOnly はオフライン再生用（ファイルを書き換えない）。
func Open(path string, readOnly bool) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(0)"
	if readOnly {
		dsn += "&mode=ro"
	} else {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("DBを開けません %s: %w", path, err)
	}
	// SQLiteは単一ライター。接続を1本に絞り "database is locked" を回避する
	db.SetMaxOpenConns(1)
	s := &Store{DB: db, path: path, readOnly: readOnly}
	if !readOnly {
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, fmt.Errorf("スキーマ適用に失敗: %w", err)
		}
		if err := migrateSwitchesAutoinc(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("DB移行に失敗: %w", err)
		}
	}
	return s, nil
}

// migrateSwitchesAutoinc は旧版DBの switches を AUTOINCREMENT 付きへ作り直す。
// 削除済みスイッチのIDが再利用されると、残存する時系列・イベントが新しい機器に紐付いてしまうため。
func migrateSwitchesAutoinc(db *sql.DB) error {
	var ddl string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='switches'`).Scan(&ddl); err != nil {
		return err
	}
	if strings.Contains(strings.ToUpper(ddl), "AUTOINCREMENT") {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var maxHist sql.NullInt64
	tx.QueryRow(`SELECT MAX(m) FROM (SELECT MAX(switch_id) m FROM port_metrics_raw UNION ALL SELECT MAX(switch_id) FROM events
		UNION ALL SELECT MAX(switch_id) FROM switch_health_raw UNION ALL SELECT MAX(switch_id) FROM device_bindings UNION ALL SELECT MAX(switch_id) FROM port_metrics_1m
		UNION ALL SELECT MAX(switch_id) FROM switch_health_1m UNION ALL SELECT MAX(id) FROM switches)`).Scan(&maxHist)
	newDDL := strings.Replace(ddl, "switches", "switches_new", 1)
	newDDL = strings.Replace(newDDL, "id INTEGER PRIMARY KEY,", "id INTEGER PRIMARY KEY AUTOINCREMENT,", 1)
	for _, q := range []string{newDDL, `INSERT INTO switches_new SELECT * FROM switches`, `DROP TABLE switches`, `ALTER TABLE switches_new RENAME TO switches`} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	// 履歴に残る最大IDより後から採番させる
	if _, err := tx.Exec(`DELETE FROM sqlite_sequence WHERE name='switches'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO sqlite_sequence(name,seq) VALUES('switches',?)`, maxHist.Int64); err != nil {
		return err
	}
	return tx.Commit()
}

// Close はDBを閉じる。
func (s *Store) Close() error { return s.DB.Close() }

func ms(t time.Time) int64     { return t.UnixMilli() }
func fromMS(v int64) time.Time { return time.UnixMilli(v).UTC() }

// ---- プロファイル ----

// Profile は現場プロファイル。
type Profile struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// EnsureDefaultProfile は初回起動時に既定プロファイルを作り、アクティブIDを返す。
func (s *Store) EnsureDefaultProfile() (int64, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT id FROM profiles WHERE active=1 LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	err = s.DB.QueryRow(`SELECT id FROM profiles ORDER BY id LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		r, err := s.DB.Exec(`INSERT INTO profiles(name,active,created_at) VALUES('既定の現場',1,?)`, time.Now().UTC().Format(time.RFC3339))
		if err != nil {
			return 0, err
		}
		return r.LastInsertId()
	}
	if err != nil {
		return 0, err
	}
	_, err = s.DB.Exec(`UPDATE profiles SET active=1 WHERE id=?`, id)
	return id, err
}

func (s *Store) Profiles() ([]Profile, error) {
	rows, err := s.DB.Query(`SELECT id,name,active FROM profiles ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Active); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateProfile(name string) (int64, error) {
	r, err := s.DB.Exec(`INSERT INTO profiles(name,active,created_at) VALUES(?,0,?)`, name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *Store) ActivateProfile(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.Exec(`UPDATE profiles SET active=CASE WHEN id=? THEN 1 ELSE 0 END`, id)
	if err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM profiles WHERE id=?`, id).Scan(&n); err != nil || n == 0 {
		return fmt.Errorf("プロファイル %d が存在しません", id)
	}
	_ = r
	return tx.Commit()
}

// ProfileJSON はプロファイルのJSON列（settings/topology_layout/watchlist）を読み書きする。
func (s *Store) ProfileJSON(id int64, col string, dst any) error {
	if !validJSONCol(col) {
		return fmt.Errorf("不正な列 %s", col)
	}
	var raw string
	if err := s.DB.QueryRow(`SELECT `+col+` FROM profiles WHERE id=?`, id).Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), dst)
}

func (s *Store) SetProfileJSON(id int64, col string, v any) error {
	if !validJSONCol(col) {
		return fmt.Errorf("不正な列 %s", col)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`UPDATE profiles SET `+col+`=? WHERE id=?`, string(b), id)
	return err
}

func validJSONCol(c string) bool {
	return c == "settings_json" || c == "topology_layout_json" || c == "watchlist_json"
}

func (s *Store) SetBackground(id int64, data []byte, ctype string) error {
	_, err := s.DB.Exec(`UPDATE profiles SET background=?, background_type=? WHERE id=?`, data, ctype, id)
	return err
}

func (s *Store) Background(id int64) ([]byte, string, error) {
	var b []byte
	var t sql.NullString
	err := s.DB.QueryRow(`SELECT background, background_type FROM profiles WHERE id=?`, id).Scan(&b, &t)
	return b, t.String, err
}

// ---- スイッチ ----

func (s *Store) Switches(profileID int64) ([]model.SwitchConfig, error) {
	rows, err := s.DB.Query(`SELECT id,profile_id,name,group_tag,ip,snmp_port,community_ro,enabled,web_url FROM switches WHERE profile_id=? ORDER BY id`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.SwitchConfig
	for rows.Next() {
		var c model.SwitchConfig
		if err := rows.Scan(&c.ID, &c.ProfileID, &c.Name, &c.GroupTag, &c.IP, &c.SNMPPort, &c.Community, &c.Enabled, &c.WebURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SaveSwitch(c *model.SwitchConfig) error {
	if c.SNMPPort == 0 {
		c.SNMPPort = 161
	}
	if c.ID == 0 {
		r, err := s.DB.Exec(`INSERT INTO switches(profile_id,name,group_tag,ip,snmp_port,community_ro,enabled,web_url) VALUES(?,?,?,?,?,?,?,?)`,
			c.ProfileID, c.Name, c.GroupTag, c.IP, c.SNMPPort, c.Community, c.Enabled, c.WebURL)
		if err != nil {
			return err
		}
		c.ID, err = r.LastInsertId()
		return err
	}
	_, err := s.DB.Exec(`UPDATE switches SET name=?,group_tag=?,ip=?,snmp_port=?,community_ro=?,enabled=?,web_url=? WHERE id=?`,
		c.Name, c.GroupTag, c.IP, c.SNMPPort, c.Community, c.Enabled, c.WebURL, c.ID)
	return err
}

func (s *Store) DeleteSwitch(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM switches WHERE id=?`, id)
	if err == nil {
		_, err = s.DB.Exec(`DELETE FROM ports WHERE switch_id=?`, id)
	}
	return err
}

func (s *Store) SetSwitchCaps(id int64, caps map[string]string, modelName, fw, oid string) {
	b, _ := json.Marshal(caps)
	s.DB.Exec(`UPDATE switches SET oid_capabilities_json=?, model=?, fw_version=?, sys_object_id=? WHERE id=?`, string(b), modelName, fw, oid, id)
}

// PortSetting はポートの手動設定。
type PortSetting struct {
	Excluded      bool   `json:"excluded"`
	ExpectedSpeed int    `json:"expected_speed_mbps"`
	AVRole        string `json:"av_role"`
	Memo          string `json:"memo"`
}

func (s *Store) PortSettings(switchID int64) (map[int]PortSetting, error) {
	rows, err := s.DB.Query(`SELECT if_index,monitor_excluded,expected_speed_mbps,expected_av_role,manual_memo FROM ports WHERE switch_id=?`, switchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]PortSetting{}
	for rows.Next() {
		var ix int
		var p PortSetting
		if err := rows.Scan(&ix, &p.Excluded, &p.ExpectedSpeed, &p.AVRole, &p.Memo); err != nil {
			return nil, err
		}
		out[ix] = p
	}
	return out, rows.Err()
}

func (s *Store) SavePortSetting(switchID int64, ifIndex int, p PortSetting) error {
	_, err := s.DB.Exec(`INSERT INTO ports(switch_id,if_index,monitor_excluded,expected_speed_mbps,expected_av_role,manual_memo) VALUES(?,?,?,?,?,?)
		ON CONFLICT(switch_id,if_index) DO UPDATE SET monitor_excluded=excluded.monitor_excluded, expected_speed_mbps=excluded.expected_speed_mbps,
		expected_av_role=excluded.expected_av_role, manual_memo=excluded.manual_memo`, switchID, ifIndex, p.Excluded, p.ExpectedSpeed, p.AVRole, p.Memo)
	return err
}

// Alias は手動ラベル。
type Alias struct {
	ID        int64  `json:"id"`
	MatchType string `json:"match_type"`
	MatchKey  string `json:"match_key"`
	Label     string `json:"label"`
	Note      string `json:"note"`
}

func (s *Store) Aliases(profileID int64) ([]Alias, error) {
	rows, err := s.DB.Query(`SELECT id,match_type,match_key,label,note FROM device_aliases WHERE profile_id=?`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alias
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.ID, &a.MatchType, &a.MatchKey, &a.Label, &a.Note); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) SaveAlias(profileID int64, a Alias) error {
	if a.Label == "" {
		_, err := s.DB.Exec(`DELETE FROM device_aliases WHERE profile_id=? AND match_type=? AND match_key=?`, profileID, a.MatchType, a.MatchKey)
		return err
	}
	_, err := s.DB.Exec(`INSERT INTO device_aliases(profile_id,match_type,match_key,label,note) VALUES(?,?,?,?,?)
		ON CONFLICT(profile_id,match_type,match_key) DO UPDATE SET label=excluded.label, note=excluded.note`, profileID, a.MatchType, a.MatchKey, a.Label, a.Note)
	return err
}

// AliasImportResult はCSV一括登録の集計。
type AliasImportResult struct {
	Added     int `json:"added"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Deleted   int `json:"deleted"` // 置換モードで消えるMAC別名
}

// ApplyMACAliases はMAC別名を一括で反映する。commit=false なら集計だけ返し何も書かない（プレビュー）。
// replace=true ならファイルにないMAC別名を削除する（ポート別名は対象外）。全件を1トランザクションで行い、
// 途中失敗で半端な状態を残さない。
func (s *Store) ApplyMACAliases(profileID int64, rows []Alias, replace, commit bool) (AliasImportResult, error) {
	var res AliasImportResult
	cur, err := s.Aliases(profileID)
	if err != nil {
		return res, err
	}
	existing := map[string]Alias{}
	for _, a := range cur {
		if a.MatchType == "mac" {
			existing[a.MatchKey] = a
		}
	}
	inFile := map[string]bool{}
	for _, r := range rows {
		inFile[r.MatchKey] = true
		old, ok := existing[r.MatchKey]
		switch {
		case !ok:
			res.Added++
		case old.Label == r.Label && old.Note == r.Note:
			res.Unchanged++
		default:
			res.Updated++
		}
	}
	var del []string
	if replace {
		for k := range existing {
			if !inFile[k] {
				del = append(del, k)
			}
		}
	}
	res.Deleted = len(del)
	if !commit {
		return res, nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	for _, k := range del {
		if _, err := tx.Exec(`DELETE FROM device_aliases WHERE profile_id=? AND match_type='mac' AND match_key=?`, profileID, k); err != nil {
			return res, err
		}
	}
	for _, r := range rows {
		if _, err := tx.Exec(`INSERT INTO device_aliases(profile_id,match_type,match_key,label,note) VALUES(?,'mac',?,?,?)
			ON CONFLICT(profile_id,match_type,match_key) DO UPDATE SET label=excluded.label, note=excluded.note`, profileID, r.MatchKey, r.Label, r.Note); err != nil {
			return res, err
		}
	}
	return res, tx.Commit()
}

// LastSeen は機器の最終観測（時刻と、そのとき使っていたIP）。
type LastSeen struct {
	TS time.Time
	IP string
}

// LastSeenByMAC はプロファイル内のスイッチで観測されたMACごとの最終観測（at 以前）。
// 見えていない登録済み機器の表示・Ping用。再生では at に再生時刻を渡し、未来の観測を混ぜない。
// 相関サブクエリを使わず1回の走査で集計する（履歴が多くても単一DB接続を長く占有しないため）。
func (s *Store) LastSeenByMAC(profileID int64, at time.Time) (map[string]LastSeen, error) {
	cut := ms(at)
	rows, err := s.DB.Query(`SELECT b.mac, b.last_seen_ts, b.ip FROM device_bindings b JOIN switches w ON w.id=b.switch_id
		WHERE w.profile_id=? AND b.first_seen_ts<=?`, profileID, cut)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]LastSeen{}
	for rows.Next() {
		var mac string
		var ts int64
		var ip sql.NullString
		if err := rows.Scan(&mac, &ts, &ip); err != nil {
			return nil, err
		}
		if ts > cut {
			ts = cut // at の時点ではまだ接続中だった（その時点が最終確認）
		}
		mac = strings.ToUpper(mac)
		if cur, ok := out[mac]; !ok || ms(cur.TS) < ts || (ms(cur.TS) == ts && cur.IP == "") {
			out[mac] = LastSeen{TS: fromMS(ts), IP: ip.String}
		}
	}
	return out, rows.Err()
}

// ProfileOfSwitch はスイッチの属するプロファイル（再生データで別名を引くため）。
func (s *Store) ProfileOfSwitch(switchID int64) (int64, error) {
	var id int64
	err := s.DB.QueryRow(`SELECT profile_id FROM switches WHERE id=?`, switchID).Scan(&id)
	return id, err
}

// ---- 時系列 ----

// InsertPortMetrics はFastティック1回分を1トランザクションで書き込む。
// withMedium=false のときは Medium周期で更新される増分（FCS・CoS・PAUSE）を0として書く（多重集計防止）。
func (s *Store) InsertPortMetrics(ts time.Time, switchID int64, ports []*model.Port, withMedium bool) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO port_metrics_raw VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, p := range ports {
		var poe, sfp any
		if p.PoE != nil {
			poe = p.PoE.PowerW * 1000
		}
		if p.SFP != nil {
			sfp = p.SFP.RxDBm
		}
		fcs, cos, pause := p.FCSDelta, p.CosDropDelta, p.PauseDelta
		if !withMedium {
			fcs, cos, pause = 0, 0, 0
		}
		if _, err := st.Exec(ms(ts), switchID, p.IfIndex, p.Oper == "up", p.SpeedMbps, p.RxBps, p.TxBps, p.RxMcPps, p.TxMcPps, p.RxBcPps,
			p.ErrDelta, fcs, p.DiscardDelta, cos, pause, poe, sfp); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) InsertHealth(ts time.Time, st *model.SwitchState) error {
	h := st.Health
	_, err := s.DB.Exec(`INSERT INTO switch_health_raw VALUES(?,?,?,?,?,?,?,?,?,?,?)`, ms(ts), st.ID, h.CPU5s, h.CPU60s, h.CPU300s, h.MemFreePct,
		h.MbufUsedPct, h.MbufFailDelta, h.TempMaxC, h.PoEUsedW, st.UptimeSec)
	return err
}

// Point は時系列1点（APIの points 要素）。
type Point map[string]any

// PortSeries は期間に応じて生データ（6時間以内）または1分集約を返す。
func (s *Store) PortSeries(switchID int64, ifIndex int, from, to time.Time) (string, []Point, error) {
	if to.Sub(from) <= 6*time.Hour {
		rows, err := s.DB.Query(`SELECT ts,rx_bps,tx_bps,rx_mc_pps,tx_mc_pps,err_delta,fcs_delta,discard_delta,cos_drop_delta,pause_delta,poe_mw,sfp_rx_dbm
			FROM port_metrics_raw WHERE switch_id=? AND if_index=? AND ts BETWEEN ? AND ? ORDER BY ts`, switchID, ifIndex, ms(from), ms(to))
		if err != nil {
			return "", nil, err
		}
		pts, err := scanPoints(rows, true)
		if err != nil || len(pts) > 0 {
			return "raw", downsample(pts, 3600), err
		}
	}
	rows, err := s.DB.Query(`SELECT ts_minute,rx_bps_avg,tx_bps_avg,rx_mc_pps_avg,tx_mc_pps_avg,err_sum,fcs_sum,discard_sum,cos_drop_sum,pause_sum,poe_mw_avg,sfp_rx_dbm_min
		FROM port_metrics_1m WHERE switch_id=? AND if_index=? AND ts_minute BETWEEN ? AND ? ORDER BY ts_minute`, switchID, ifIndex, ms(from), ms(to))
	if err != nil {
		return "", nil, err
	}
	pts, err := scanPoints(rows, true)
	return "1m", downsample(pts, 3600), err
}

func scanPoints(rows *sql.Rows, port bool) ([]Point, error) {
	defer rows.Close()
	cols := []string{"rx_bps", "tx_bps", "rx_mc_pps", "tx_mc_pps", "err_delta", "fcs_delta", "discard_delta", "cos_drop_delta", "pause_delta", "poe_w", "sfp_rx_dbm"}
	if !port {
		cols = []string{"cpu_5s", "mem_free_pct", "mbuf_used_pct", "temp_max_c", "poe_used_w"}
	}
	var out []Point
	for rows.Next() {
		var ts int64
		vals := make([]sql.NullFloat64, len(cols))
		dst := []any{&ts}
		for i := range vals {
			dst = append(dst, &vals[i])
		}
		if err := rows.Scan(dst...); err != nil {
			return nil, err
		}
		p := Point{"ts": fromMS(ts)}
		for i, c := range cols {
			if vals[i].Valid {
				v := vals[i].Float64
				if c == "poe_w" {
					v /= 1000
				}
				p[c] = v
			} else {
				p[c] = nil
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// downsample は点数上限を超える場合に等間隔で間引く（フロントのメモリ保護）。
func downsample(p []Point, max int) []Point {
	if len(p) <= max {
		return p
	}
	step := float64(len(p)) / float64(max)
	out := make([]Point, 0, max)
	for i := 0.0; int(i) < len(p); i += step {
		out = append(out, p[int(i)])
	}
	return out
}

func (s *Store) SwitchSeries(switchID int64, from, to time.Time) (string, []Point, error) {
	if to.Sub(from) <= 6*time.Hour {
		rows, err := s.DB.Query(`SELECT ts,cpu_5s,mem_free_pct,mbuf_used_pct,temp_max_c,poe_total_w FROM switch_health_raw WHERE switch_id=? AND ts BETWEEN ? AND ? ORDER BY ts`, switchID, ms(from), ms(to))
		if err != nil {
			return "", nil, err
		}
		pts, err := scanPoints(rows, false)
		if err != nil || len(pts) > 0 {
			return "raw", downsample(pts, 3600), err
		}
	}
	rows, err := s.DB.Query(`SELECT ts_minute,cpu_5s_avg,mem_free_pct_min,mbuf_used_pct_max,temp_max_c,poe_total_w_avg FROM switch_health_1m WHERE switch_id=? AND ts_minute BETWEEN ? AND ? ORDER BY ts_minute`, switchID, ms(from), ms(to))
	if err != nil {
		return "", nil, err
	}
	pts, err := scanPoints(rows, false)
	return "1m", downsample(pts, 3600), err
}

// Rollup は確定済みの分（現在分の手前まで）を1分集約へ書き込む。冪等（INSERT OR REPLACE）。
func (s *Store) Rollup(ctx context.Context, upTo time.Time) error {
	end := upTo.Truncate(time.Minute)
	var last int64
	s.DB.QueryRow(`SELECT COALESCE((SELECT CAST(v AS INTEGER) FROM meta WHERE k='rollup_until'),0)`).Scan(&last)
	if last == 0 {
		s.DB.QueryRow(`SELECT COALESCE(MIN(ts),0) FROM port_metrics_raw`).Scan(&last)
		last = last - last%60000
	}
	if last == 0 || last >= ms(end) {
		return nil
	}
	_, err := s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO port_metrics_1m
		SELECT (ts/60000)*60000, switch_id, if_index, AVG(rx_bps), MAX(rx_bps), AVG(tx_bps), MAX(tx_bps), AVG(rx_mc_pps), AVG(tx_mc_pps),
		SUM(err_delta), SUM(fcs_delta), SUM(discard_delta), SUM(cos_drop_delta), SUM(pause_delta), AVG(poe_mw), MIN(sfp_rx_dbm)
		FROM port_metrics_raw WHERE ts >= ? AND ts < ? GROUP BY 1,2,3`, last, ms(end))
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO switch_health_1m
		SELECT (ts/60000)*60000, switch_id, AVG(cpu_5s), MAX(cpu_5s), MIN(mem_free_pct), MAX(mbuf_used_pct), MAX(temp_max_c), AVG(poe_total_w)
		FROM switch_health_raw WHERE ts >= ? AND ts < ? GROUP BY 1,2`, last, ms(end))
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT OR REPLACE INTO meta(k,v) VALUES('rollup_until',?)`, fmt.Sprint(ms(end)))
	return err
}

// Retention は保持期間設定（仕様2.3-4）。
type Retention struct {
	Raw, Agg, Events time.Duration
	MaxDBBytes       int64
}

var DefaultRetention = Retention{Raw: 36 * time.Hour, Agg: 30 * 24 * time.Hour, Events: 90 * 24 * time.Hour, MaxDBBytes: 2 << 30}

// Purge は保持期間外を削除し、DBサイズ上限超過時は生データを古い順に追加削除する。
func (s *Store) Purge(ctx context.Context, r Retention, now time.Time) error {
	stmts := []struct {
		q   string
		cut time.Duration
	}{
		{`DELETE FROM port_metrics_raw WHERE ts < ?`, r.Raw},
		{`DELETE FROM switch_health_raw WHERE ts < ?`, r.Raw},
		{`DELETE FROM state_snapshots WHERE ts < ?`, r.Agg},
		{`DELETE FROM port_metrics_1m WHERE ts_minute < ?`, r.Agg},
		{`DELETE FROM switch_health_1m WHERE ts_minute < ?`, r.Agg},
		{`DELETE FROM events WHERE ts < ? AND category <> 'MARKER'`, r.Events},
	}
	for _, st := range stmts {
		if _, err := s.DB.ExecContext(ctx, st.q, ms(now.Add(-st.cut))); err != nil {
			return err
		}
	}
	if fi, err := os.Stat(s.path); err == nil && r.MaxDBBytes > 0 && fi.Size() > r.MaxDBBytes {
		// 生データの古い半分を削除してから VACUUM でファイルを縮める
		var min, max int64
		s.DB.QueryRow(`SELECT COALESCE(MIN(ts),0), COALESCE(MAX(ts),0) FROM port_metrics_raw`).Scan(&min, &max)
		if max > min {
			s.DB.ExecContext(ctx, `DELETE FROM port_metrics_raw WHERE ts < ?`, min+(max-min)/2)
			s.DB.ExecContext(ctx, `DELETE FROM state_snapshots WHERE ts < ?`, min+(max-min)/2)
		}
		if _, err := s.DB.ExecContext(ctx, `VACUUM`); err != nil {
			return fmt.Errorf("VACUUM失敗: %w", err)
		}
	}
	return nil
}

// ---- イベント ----

// InsertEvent はイベントを保存しIDを返す。Dedupe一致で既存の場合は id=0, inserted=false。
func (s *Store) InsertEvent(e *model.Event) (bool, error) {
	det, _ := json.Marshal(e.Detail)
	var dk any
	if e.Dedupe != "" {
		dk = e.Dedupe
	}
	r, err := s.DB.Exec(`INSERT OR IGNORE INTO events(ts,switch_id,switch_name,if_index,category,severity,code,title,detail_json,hint_text,acked,dedupe_key)
		VALUES(?,?,?,?,?,?,?,?,?,?,0,?)`, ms(e.TS), e.SwitchID, e.SwitchName, e.IfIndex, e.Category, e.Severity, e.Code, e.Title, string(det), e.Hint, dk)
	if err != nil {
		return false, err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return false, nil
	}
	e.ID, _ = r.LastInsertId()
	return true, nil
}

// EventFilter は検索条件。
type EventFilter struct {
	Category, Severity, Q string
	SwitchID              int64
	From, To              time.Time
	Limit                 int
}

func (s *Store) Events(f EventFilter) ([]model.Event, error) {
	var conds []string
	var args []any
	if f.Category != "" {
		conds, args = append(conds, "category=?"), append(args, f.Category)
	}
	if f.Severity != "" {
		conds, args = append(conds, "severity=?"), append(args, f.Severity)
	}
	if f.SwitchID != 0 {
		conds, args = append(conds, "switch_id=?"), append(args, f.SwitchID)
	}
	if f.Q != "" {
		conds, args = append(conds, "(title LIKE ? OR detail_json LIKE ? OR switch_name LIKE ?)"), append(args, "%"+f.Q+"%", "%"+f.Q+"%", "%"+f.Q+"%")
	}
	if !f.From.IsZero() {
		conds, args = append(conds, "ts>=?"), append(args, ms(f.From))
	}
	if !f.To.IsZero() {
		conds, args = append(conds, "ts<=?"), append(args, ms(f.To))
	}
	q := `SELECT id,ts,switch_id,switch_name,if_index,category,severity,code,title,detail_json,hint_text,acked FROM events`
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	if f.Limit <= 0 || f.Limit > 10000 {
		f.Limit = 500
	}
	q += fmt.Sprintf(" ORDER BY ts DESC, id DESC LIMIT %d", f.Limit)
	rows, err := s.DB.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Event{}
	for rows.Next() {
		var e model.Event
		var ts int64
		var det string
		if err := rows.Scan(&e.ID, &ts, &e.SwitchID, &e.SwitchName, &e.IfIndex, &e.Category, &e.Severity, &e.Code, &e.Title, &det, &e.Hint, &e.Acked); err != nil {
			return nil, err
		}
		e.TS = fromMS(ts)
		json.Unmarshal([]byte(det), &e.Detail)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Ack(id int64) error {
	_, err := s.DB.Exec(`UPDATE events SET acked=1, acked_at=? WHERE id=?`, ms(time.Now()), id)
	return err
}

func (s *Store) AckAll() error {
	_, err := s.DB.Exec(`UPDATE events SET acked=1, acked_at=? WHERE acked=0`, ms(time.Now()))
	return err
}

// ---- スナップショット・ベースライン ----

func gz(v any) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func ungz(b []byte, dst any) error {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, 256<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// InsertSnapshot は全スイッチ状態をgzip圧縮して保存する（タイムマシン再生用）。
func (s *Store) InsertSnapshot(ts time.Time, trigger string, states []*model.SwitchState) error {
	b, err := gz(states)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO state_snapshots(ts,trigger,state_gz) VALUES(?,?,?)`, ms(ts), trigger, b)
	return err
}

// SnapshotAt は ts 以前で最新のスナップショットを返す（無ければ最古）。
func (s *Store) SnapshotAt(ts time.Time) (time.Time, []*model.SwitchState, error) {
	var t int64
	var b []byte
	// 未来の状態を過去に見せないため、ts以前が無ければ該当なしとする
	err := s.DB.QueryRow(`SELECT ts,state_gz FROM state_snapshots WHERE ts<=? ORDER BY ts DESC LIMIT 1`, ms(ts)).Scan(&t, &b)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil, fmt.Errorf("指定時刻（%s）以前の状態スナップショットがありません", ts.Format(time.RFC3339))
	}
	if err != nil {
		return time.Time{}, nil, err
	}
	var out []*model.SwitchState
	err = ungz(b, &out)
	return fromMS(t), out, err
}

// TimeRange はスナップショット・イベントの期間。
func (s *Store) TimeRange() (time.Time, time.Time) {
	var a, b int64
	s.DB.QueryRow(`SELECT COALESCE(MIN(ts),0), COALESCE(MAX(ts),0) FROM (SELECT ts FROM state_snapshots UNION ALL SELECT ts FROM events)`).Scan(&a, &b)
	return fromMS(a), fromMS(b)
}

type BaselineInfo struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	Captured time.Time `json:"captured_ts"`
}

func (s *Store) SaveBaseline(profileID int64, name string, states []*model.SwitchState) (int64, error) {
	b, err := gz(states)
	if err != nil {
		return 0, err
	}
	r, err := s.DB.Exec(`INSERT INTO baselines(profile_id,name,captured_ts,snapshot_gz) VALUES(?,?,?,?)`, profileID, name, ms(time.Now()), b)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *Store) Baselines(profileID int64) ([]BaselineInfo, error) {
	rows, err := s.DB.Query(`SELECT id,name,captured_ts FROM baselines WHERE profile_id=? ORDER BY id DESC`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BaselineInfo{}
	for rows.Next() {
		var b BaselineInfo
		var t int64
		if err := rows.Scan(&b.ID, &b.Name, &t); err != nil {
			return nil, err
		}
		b.Captured = fromMS(t)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) Baseline(id int64) ([]*model.SwitchState, error) {
	var b []byte
	if err := s.DB.QueryRow(`SELECT snapshot_gz FROM baselines WHERE id=?`, id).Scan(&b); err != nil {
		return nil, err
	}
	var out []*model.SwitchState
	return out, ungz(b, &out)
}

func (s *Store) DeleteBaseline(id int64) error {
	_, err := s.DB.Exec(`DELETE FROM baselines WHERE id=?`, id)
	return err
}

// UpsertBinding は接続履歴（機器×ポート）を更新する。
func (s *Store) UpsertBinding(switchID int64, ifIndex int, d model.Device, now time.Time) {
	r, err := s.DB.Exec(`UPDATE device_bindings SET last_seen_ts=?, ip=CASE WHEN ?<>'' THEN ? ELSE ip END, lldp_sys_name=CASE WHEN ?<>'' THEN ? ELSE lldp_sys_name END
		WHERE switch_id=? AND if_index=? AND mac=? AND is_active=1`, ms(now), d.IP, d.IP, d.LLDPName, d.LLDPName, switchID, ifIndex, d.MAC)
	if err != nil {
		return
	}
	if n, _ := r.RowsAffected(); n == 0 {
		s.DB.Exec(`INSERT INTO device_bindings(switch_id,if_index,mac,oui_vendor,ip,vlan_id,lldp_sys_name,first_seen_ts,last_seen_ts,is_active) VALUES(?,?,?,?,?,?,?,?,?,1)`,
			switchID, ifIndex, d.MAC, d.Vendor, d.IP, d.VLAN, d.LLDPName, ms(now), ms(now))
	}
}

// DeactivateBinding は消失した機器の履歴を非アクティブにする。
func (s *Store) DeactivateBinding(switchID int64, ifIndex int, mac string) {
	s.DB.Exec(`UPDATE device_bindings SET is_active=0 WHERE switch_id=? AND if_index=? AND mac=? AND is_active=1`, switchID, ifIndex, mac)
}

// Connections は接続履歴（CSV出力用）。
func (s *Store) Connections(from, to time.Time) ([]map[string]any, error) {
	rows, err := s.DB.Query(`SELECT switch_id,if_index,mac,oui_vendor,ip,vlan_id,lldp_sys_name,first_seen_ts,last_seen_ts,is_active FROM device_bindings
		WHERE last_seen_ts>=? AND first_seen_ts<=? ORDER BY first_seen_ts`, ms(from), ms(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var sw, ix, vlan int64
		var mac, ven, ip, ln string
		var f, l int64
		var act bool
		if err := rows.Scan(&sw, &ix, &mac, &ven, &ip, &vlan, &ln, &f, &l, &act); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"switch_id": sw, "if_index": ix, "mac": mac, "vendor": ven, "ip": ip, "vlan": vlan, "lldp_name": ln,
			"first_seen": fromMS(f), "last_seen": fromMS(l), "active": act})
	}
	return out, rows.Err()
}
