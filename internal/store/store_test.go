package store

import (
	"bytes"
	"context"
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
