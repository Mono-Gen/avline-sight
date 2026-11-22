package store

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func TestStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "a.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pid, err := s.EnsureDefaultProfile()
	if err != nil || pid == 0 {
		t.Fatal(err)
	}
	sw := model.SwitchConfig{ProfileID: pid, Name: "SW1", IP: "10.0.0.1", Community: "secret", Enabled: true}
	if err := s.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-10 * time.Minute).Truncate(time.Minute)
	for i := 0; i < 120; i++ {
		ts := base.Add(time.Duration(i) * 3 * time.Second)
		if err := s.InsertPortMetrics(ts, sw.ID, []*model.Port{{IfIndex: 1, Oper: "up", RxBps: 1000, ErrDelta: 1}}, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Rollup(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	res, pts, err := s.PortSeries(sw.ID, 1, base.Add(-time.Minute), time.Now())
	if err != nil || res != "raw" || len(pts) != 120 {
		t.Fatalf("raw series: %s %d %v", res, len(pts), err)
	}
	var n, sum int
	s.DB.QueryRow(`SELECT COUNT(*), SUM(err_sum) FROM port_metrics_1m`).Scan(&n, &sum)
	if n != 6 || sum != 120 {
		t.Fatalf("1分集約: rows=%d err_sum=%d (want 6,120)", n, sum)
	}
	// 冪等: 2回目のRollupで重複しない
	s.Rollup(context.Background(), time.Now())
	s.DB.QueryRow(`SELECT SUM(err_sum) FROM port_metrics_1m`).Scan(&sum)
	if sum != 120 {
		t.Fatalf("Rollupが冪等でない: %d", sum)
	}

	// イベントの重複排除
	e := model.Event{TS: time.Now(), SwitchID: sw.ID, Category: "SYSLOG", Severity: "INFO", Code: "SWITCH_LOG", Title: "x", Dedupe: "log:1:x"}
	ok1, _ := s.InsertEvent(&e)
	e2 := e
	ok2, _ := s.InsertEvent(&e2)
	if !ok1 || ok2 {
		t.Fatalf("dedupe: %v %v", ok1, ok2)
	}
	s.InsertEvent(&model.Event{TS: time.Now(), Category: "MARKER", Severity: "INFO", Code: "MARKER", Title: "2幕冒頭 音切れ"})
	evs, _ := s.Events(EventFilter{Q: "音切れ"})
	if len(evs) != 1 {
		t.Fatalf("検索結果 %d", len(evs))
	}

	st := []*model.SwitchState{{ID: sw.ID, Name: "SW1", Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1"}}}}
	legacyAt := time.Now().Add(-time.Second).Truncate(time.Millisecond)
	legacy, err := gz(st)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO state_snapshots(ts,trigger,state_gz) VALUES(?,?,?)`, ms(legacyAt), "periodic", legacy); err != nil {
		t.Fatal(err)
	}
	snapshotAlerts := `[{"event_id":1,"switch_id":0,"severity":"WARNING"}]`
	if err := s.InsertSnapshot(time.Now(), "periodic", Snapshot{States: st, Alerts: []byte(snapshotAlerts)}); err != nil {
		t.Fatal(err)
	}

	// 書出→読込
	var buf bytes.Buffer
	if err := s.ExportPack(&buf, dir, base.Add(-time.Hour), time.Now().Add(time.Minute), "test"); err != nil {
		t.Fatal(err)
	}
	zp := filepath.Join(dir, "x.avsight")
	os.WriteFile(zp, buf.Bytes(), 0o644)
	rd := filepath.Join(dir, "replay")
	os.MkdirAll(rd, 0o755)
	r, man, err := ImportPack(zp, rd)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(man.Switches) != 1 {
		t.Errorf("manifest: %+v", man)
	}
	_, snap, err := r.SnapshotAt(time.Now())
	if err != nil || len(snap) != 1 || snap[0].Ports[0].Name != "1/0/1" {
		t.Fatalf("snapshot: %v %v", snap, err)
	}
	_, recorded, err := r.SnapshotWithAlertsAt(time.Now())
	if err != nil || string(recorded.Alerts) != snapshotAlerts {
		t.Fatalf("snapshot alerts: %s (%v)", recorded.Alerts, err)
	}
	_, old, err := r.SnapshotWithAlertsAt(legacyAt)
	if err != nil || len(old.States) != 1 || old.States[0].ID != sw.ID || old.Alerts != nil {
		t.Fatalf("legacy snapshot: %+v (%v)", old, err)
	}
	var comm string
	r.DB.QueryRow(`SELECT community_ro FROM switches`).Scan(&comm)
	if comm != "****" {
		t.Errorf("書出にコミュニティが平文で残っている: %q", comm)
	}
	if _, err := r.DB.Exec(`DELETE FROM events`); err == nil {
		t.Error("再生DBが書き込み可能になっている")
	}
}

// 監査2周目A-1の回帰テスト: 旧スキーマDBでも移行後はIDが再利用されない
func TestMigrateSwitchesAutoinc(t *testing.T) {
	p := filepath.Join(t.TempDir(), "old.db")
	old, err := sql.Open("sqlite", "file:"+p)
	if err != nil {
		t.Fatal(err)
	}
	old.Exec(`CREATE TABLE switches (id INTEGER PRIMARY KEY, profile_id INTEGER NOT NULL, name TEXT NOT NULL, group_tag TEXT NOT NULL DEFAULT '',
  ip TEXT NOT NULL, snmp_port INTEGER NOT NULL DEFAULT 161, community_ro TEXT NOT NULL DEFAULT 'public',
  web_url TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '', fw_version TEXT NOT NULL DEFAULT '',
  sys_object_id TEXT NOT NULL DEFAULT '', oid_capabilities_json TEXT NOT NULL DEFAULT '{}', enabled INTEGER NOT NULL DEFAULT 1)`)
	old.Exec(`INSERT INTO switches(id,profile_id,name,ip) VALUES(1,1,'A','10.0.0.1'),(2,1,'B','10.0.0.2')`)
	old.Close()
	s, err := Open(p, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.InsertEvent(&model.Event{TS: time.Now(), SwitchID: 2, Category: "ANOMALY", Severity: "INFO", Code: "X", Title: "旧Bの履歴"})
	s.DeleteSwitch(2)
	n := model.SwitchConfig{ProfileID: 1, Name: "C", IP: "10.0.0.3", Enabled: true}
	if err := s.SaveSwitch(&n); err != nil {
		t.Fatal(err)
	}
	if n.ID <= 2 {
		t.Fatalf("削除済みIDが再利用された: %d", n.ID)
	}
}

// CSV一括登録: プレビューは書き込まない／置換はMAC別名だけを消す／1トランザクションで反映
func TestApplyMACAliases(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pid, _ := s.EnsureDefaultProfile()
	s.SaveAlias(pid, Alias{MatchType: "mac", MatchKey: "AA:00:00:00:00:01", Label: "旧名"})
	s.SaveAlias(pid, Alias{MatchType: "mac", MatchKey: "AA:00:00:00:00:02", Label: "同じ"})
	s.SaveAlias(pid, Alias{MatchType: "mac", MatchKey: "AA:00:00:00:00:09", Label: "ファイルにない"})
	s.SaveAlias(pid, Alias{MatchType: "port", MatchKey: "1:5", Label: "ポート名"})
	in := []Alias{
		{MatchType: "mac", MatchKey: "AA:00:00:00:00:01", Label: "新名"},
		{MatchType: "mac", MatchKey: "AA:00:00:00:00:02", Label: "同じ"},
		{MatchType: "mac", MatchKey: "AA:00:00:00:00:03", Label: "追加"},
	}
	res, err := s.ApplyMACAliases(pid, in, true, false)
	if err != nil || res != (AliasImportResult{Added: 1, Updated: 1, Unchanged: 1, Deleted: 1}) {
		t.Fatalf("プレビュー集計: %+v %v", res, err)
	}
	if al, _ := s.Aliases(pid); len(al) != 4 {
		t.Fatalf("プレビューで書き込まれた: %d件", len(al))
	}
	if _, err := s.ApplyMACAliases(pid, in, true, true); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	al, _ := s.Aliases(pid)
	for _, a := range al {
		got[a.MatchType+"|"+a.MatchKey] = a.Label
	}
	want := map[string]string{"mac|AA:00:00:00:00:01": "新名", "mac|AA:00:00:00:00:02": "同じ", "mac|AA:00:00:00:00:03": "追加", "port|1:5": "ポート名"}
	if len(got) != len(want) {
		t.Fatalf("置換後: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("置換後 %s = %q（期待 %q）", k, got[k], v)
		}
	}
	// 追加/上書きモードは既存を消さない
	if res, _ := s.ApplyMACAliases(pid, in[:1], false, true); res.Deleted != 0 {
		t.Fatalf("mergeで削除: %+v", res)
	}
	if al, _ := s.Aliases(pid); len(al) != 4 {
		t.Fatalf("merge後: %d件", len(al))
	}
}

// 監査指摘#3/#8: 最終観測は現場（プロファイル）内に限り、指定時刻より後の観測を混ぜない
func TestLastSeenByMACScoped(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p1, _ := s.EnsureDefaultProfile()
	p2, _ := s.CreateProfile("別現場")
	a := model.SwitchConfig{ProfileID: p1, Name: "A", IP: "10.0.0.1", Enabled: true}
	b := model.SwitchConfig{ProfileID: p2, Name: "B", IP: "10.0.0.2", Enabled: true}
	s.SaveSwitch(&a)
	s.SaveSwitch(&b)
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	mac := "AA:00:00:00:00:01"
	// 現場1: 10:00 に 10.1.1.1、消失後 12:00 に 10.1.1.2 で再接続
	s.UpsertBinding(a.ID, 1, model.Device{MAC: mac, IP: "10.1.1.1"}, t0)
	s.UpsertBinding(a.ID, 1, model.Device{MAC: mac, IP: "10.1.1.1"}, t0.Add(30*time.Minute))
	s.DeactivateBinding(a.ID, 1, mac)
	s.UpsertBinding(a.ID, 1, model.Device{MAC: mac, IP: "10.1.1.2"}, t0.Add(2*time.Hour))
	// 別現場ではもっと新しい観測
	s.UpsertBinding(b.ID, 1, model.Device{MAC: mac, IP: "192.0.2.9"}, t0.Add(5*time.Hour))

	ls, err := s.LastSeenByMAC(p1, t0.Add(6*time.Hour))
	if err != nil || ls[mac].IP != "10.1.1.2" || !ls[mac].TS.Equal(t0.Add(2*time.Hour)) {
		t.Fatalf("現在: %+v %v（別現場の観測を混ぜてはいけない）", ls[mac], err)
	}
	ls, _ = s.LastSeenByMAC(p1, t0.Add(time.Hour)) // 消失中の時点
	if ls[mac].IP != "10.1.1.1" || !ls[mac].TS.Equal(t0.Add(30*time.Minute)) {
		t.Fatalf("消失中の時点: %+v（未来の再接続で過去の記録を消してはいけない）", ls[mac])
	}
	ls, _ = s.LastSeenByMAC(p1, t0.Add(10*time.Minute)) // 接続中の時点
	if !ls[mac].TS.Equal(t0.Add(10 * time.Minute)) {
		t.Fatalf("接続中の時点の最終確認はその時刻: %+v", ls[mac])
	}
}

// 監査2周目#4: スイッチを削除しても、その現場の接続履歴から最終観測を引ける
func TestLastSeenSurvivesSwitchDelete(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "a.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pid, _ := s.EnsureDefaultProfile()
	sw := model.SwitchConfig{ProfileID: pid, Name: "A", IP: "10.0.0.1", Enabled: true}
	s.SaveSwitch(&sw)
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	s.UpsertBinding(sw.ID, 1, model.Device{MAC: "AA:00:00:00:00:01", IP: "10.1.1.1"}, t0)
	if err := s.DeleteSwitch(sw.ID); err != nil {
		t.Fatal(err)
	}
	ls, err := s.LastSeenByMAC(pid, t0.Add(time.Hour))
	if err != nil || ls["AA:00:00:00:00:01"].IP != "10.1.1.1" {
		t.Fatalf("削除後に履歴を失った: %+v %v", ls, err)
	}
}

// 旧版DB（device_bindings に profile_id 列なし）を開くと列が追加され、既存行に現場IDが入る
func TestMigrateBindingsProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := s.EnsureDefaultProfile()
	sw := model.SwitchConfig{ProfileID: pid, Name: "A", IP: "10.0.0.1", Enabled: true}
	s.SaveSwitch(&sw)
	// 旧スキーマを再現（列を落として作り直す）
	for _, q := range []string{
		`CREATE TABLE db_old (id INTEGER PRIMARY KEY, switch_id INTEGER NOT NULL, if_index INTEGER NOT NULL, mac TEXT NOT NULL,
		  oui_vendor TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL DEFAULT '', vlan_id INTEGER NOT NULL DEFAULT 0,
		  lldp_sys_name TEXT NOT NULL DEFAULT '', first_seen_ts INTEGER NOT NULL, last_seen_ts INTEGER NOT NULL, is_active INTEGER NOT NULL DEFAULT 1)`,
		`DROP TABLE device_bindings`, `ALTER TABLE db_old RENAME TO device_bindings`,
	} {
		if _, err := s.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.DB.Exec(`INSERT INTO device_bindings(switch_id,if_index,mac,ip,first_seen_ts,last_seen_ts) VALUES(?,1,'AA:00:00:00:00:01','10.1.1.1',1000,2000)`, sw.ID)
	s.Close()
	s, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ls, err := s.LastSeenByMAC(pid, time.UnixMilli(5000))
	if err != nil || ls["AA:00:00:00:00:01"].IP != "10.1.1.1" {
		t.Fatalf("移行後に現場IDで引けない: %+v %v", ls, err)
	}
}
