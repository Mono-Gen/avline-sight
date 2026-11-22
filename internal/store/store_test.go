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
	if err := s.InsertSnapshot(time.Now(), "periodic", st); err != nil {
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
