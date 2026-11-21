package store

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Manifest は .avsight の manifest.json（仕様5.3）。
type Manifest struct {
	FormatVersion int       `json:"format_version"`
	CreatedAt     time.Time `json:"created_at"`
	FromTS        time.Time `json:"from_ts"`
	ToTS          time.Time `json:"to_ts"`
	Switches      []string  `json:"switches"`
	AppVersion    string    `json:"app_version"`
}

const PackFormatVersion = 1

// ExportPack は期間内のデータだけを抽出した自己完結SQLiteを作り、manifestとともにZIPで w へ書く。
// 機器メモ・プロファイル・ベースラインは期間に関係なく同梱する。communityは除去する（漏洩防止）。
func (s *Store) ExportPack(w io.Writer, tmpDir string, from, to time.Time, appVersion string) error {
	f, err := os.CreateTemp(tmpDir, "pack-*.sqlite")
	if err != nil {
		return fmt.Errorf("一時ファイル作成に失敗: %w", err)
	}
	tmp := f.Name()
	f.Close()
	os.Remove(tmp) // 空ファイルではなくSQLiteに作らせる
	defer os.Remove(tmp)

	out, err := Open(tmp, false)
	if err != nil {
		return err
	}
	out.Close()
	conn := s.DB
	if _, err := conn.Exec(`ATTACH DATABASE ? AS pk`, tmp); err != nil {
		return fmt.Errorf("書出DBのATTACHに失敗: %w", err)
	}
	defer conn.Exec(`DETACH DATABASE pk`)
	a, b := ms(from), ms(to)
	copies := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO pk.profiles SELECT * FROM main.profiles`, nil},
		{`INSERT INTO pk.switches SELECT * FROM main.switches`, nil},
		{`UPDATE pk.switches SET community_ro='****'`, nil},
		{`INSERT INTO pk.ports SELECT * FROM main.ports`, nil},
		{`INSERT INTO pk.device_aliases SELECT * FROM main.device_aliases`, nil},
		{`INSERT INTO pk.baselines SELECT * FROM main.baselines`, nil},
		{`INSERT INTO pk.port_metrics_raw SELECT * FROM main.port_metrics_raw WHERE ts BETWEEN ? AND ?`, []any{a, b}},
		{`INSERT INTO pk.port_metrics_1m SELECT * FROM main.port_metrics_1m WHERE ts_minute BETWEEN ? AND ?`, []any{a, b}},
		{`INSERT INTO pk.switch_health_raw SELECT * FROM main.switch_health_raw WHERE ts BETWEEN ? AND ?`, []any{a, b}},
		{`INSERT INTO pk.switch_health_1m SELECT * FROM main.switch_health_1m WHERE ts_minute BETWEEN ? AND ?`, []any{a, b}},
		{`INSERT INTO pk.events SELECT * FROM main.events WHERE ts BETWEEN ? AND ?`, []any{a, b}},
		{`INSERT INTO pk.device_bindings SELECT * FROM main.device_bindings WHERE last_seen_ts>=? AND first_seen_ts<=?`, []any{a, b}},
		// 期間開始時点の状態を再現できるよう、開始直前のスナップショット1件も含める
		{`INSERT INTO pk.state_snapshots SELECT * FROM main.state_snapshots WHERE ts BETWEEN ? AND ? OR id=(SELECT id FROM main.state_snapshots WHERE ts<? ORDER BY ts DESC LIMIT 1)`, []any{a, b, a}},
	}
	for _, c := range copies {
		if _, err := conn.Exec(c.q, c.args...); err != nil {
			return fmt.Errorf("書出データのコピーに失敗: %w", err)
		}
	}
	if _, err := conn.Exec(`DETACH DATABASE pk`); err != nil {
		return err
	}
	// VACUUMで空き領域を詰める
	if o, err := Open(tmp, false); err == nil {
		o.DB.Exec(`VACUUM`)
		o.DB.Exec(`PRAGMA journal_mode=DELETE`)
		o.Close()
	}
	var names []string
	rows, err := s.DB.Query(`SELECT name FROM switches ORDER BY id`)
	if err == nil {
		for rows.Next() {
			var n string
			rows.Scan(&n)
			names = append(names, n)
		}
		rows.Close()
	}
	man := Manifest{FormatVersion: PackFormatVersion, CreatedAt: time.Now().UTC(), FromTS: from.UTC(), ToTS: to.UTC(), Switches: names, AppVersion: appVersion}

	zw := zip.NewWriter(w)
	mw, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	enc := json.NewEncoder(mw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(man); err != nil {
		return err
	}
	dw, err := zw.Create("session.sqlite")
	if err != nil {
		return err
	}
	df, err := os.Open(tmp)
	if err != nil {
		return err
	}
	defer df.Close()
	if _, err := io.Copy(dw, df); err != nil {
		return err
	}
	return zw.Close()
}

// ImportPack は .avsight を dir へ展開し、読み取り専用ストアとmanifestを返す。
func ImportPack(zipPath, dir string) (*Store, *Manifest, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, nil, fmt.Errorf(".avsightを開けません（ZIP形式ではありません）: %w", err)
	}
	defer zr.Close()
	var man Manifest
	dbPath := filepath.Join(dir, "session.sqlite")
	foundDB := false
	for _, f := range zr.File {
		switch f.Name { // パストラバーサル防止のため既知の名前のみ扱う
		case "manifest.json":
			rc, err := f.Open()
			if err != nil {
				return nil, nil, err
			}
			err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&man)
			rc.Close()
			if err != nil {
				return nil, nil, fmt.Errorf("manifest.json が不正です: %w", err)
			}
		case "session.sqlite":
			rc, err := f.Open()
			if err != nil {
				return nil, nil, err
			}
			out, err := os.Create(dbPath)
			if err != nil {
				rc.Close()
				return nil, nil, err
			}
			_, err = io.Copy(out, io.LimitReader(rc, 8<<30))
			rc.Close()
			out.Close()
			if err != nil {
				return nil, nil, err
			}
			foundDB = true
		}
	}
	if !foundDB {
		return nil, nil, fmt.Errorf(".avsight に session.sqlite が含まれていません")
	}
	if man.FormatVersion > PackFormatVersion {
		return nil, nil, fmt.Errorf("新しい形式（v%d）のため読めません。アプリを更新してください", man.FormatVersion)
	}
	st, err := Open(dbPath, true)
	if err != nil {
		return nil, nil, err
	}
	return st, &man, nil
}
