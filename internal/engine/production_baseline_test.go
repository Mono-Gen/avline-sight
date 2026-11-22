package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func TestProductionPhaseCapturesBaselineOncePerTransition(t *testing.T) {
	e, st := newTestEngine(t)
	port := &model.Port{IfIndex: 7, Name: "1/0/7", ConfigSettings: map[string]string{"max_frame": "1522"}}
	e.OnState(&model.SwitchState{ID: 1, Name: "SW", Reachable: true, Ports: []*model.Port{port}}, "slow")
	for _, tc := range []struct {
		phase string
		count int
	}{{"show", 0}, {"setup", 0}, {"rehearsal", 0}, {"show", 1}, {"show", 1}, {"setup", 1}, {"show", 2}} {
		if err := e.SetPhase(tc.phase); err != nil {
			t.Fatal(err)
		}
		baselines, err := st.Baselines(e.ProfileID())
		if err != nil || len(baselines) != tc.count {
			t.Fatalf("phase %s: want %d baselines, got %+v (%v)", tc.phase, tc.count, baselines, err)
		}
		if len(baselines) == 0 {
			continue
		}
		b := baselines[0]
		wantTimestamp := b.Captured.UTC().Add(9 * time.Hour).Truncate(time.Minute)
		suffix := strings.SplitN(b.Name, " ", 2)
		if len(suffix) != 2 {
			t.Fatalf("missing timestamp in %q", b.Name)
		}
		parsed, err := time.Parse("2006-01-02 15:04", suffix[1])
		if err != nil || parsed.After(wantTimestamp) || wantTimestamp.Sub(parsed) > time.Minute {
			t.Fatalf("want timestamp in JST near %v, got %q", wantTimestamp, b.Name)
		}
		saved, err := st.Baseline(b.ID)
		if err != nil || len(saved) != 1 || len(saved[0].Ports) != 1 || saved[0].Ports[0].ConfigSettings["max_frame"] != "1522" {
			t.Fatalf("unexpected snapshot: %+v (%v)", saved, err)
		}
	}
	if n := codes(t, st)["BASELINE_SAVED"]; n != 2 {
		t.Fatalf("want two baseline saved events, got %d", n)
	}
	baselines, err := st.Baselines(e.ProfileID())
	if err != nil || len(baselines) != 2 {
		t.Fatalf("load latest baseline: %+v (%v)", baselines, err)
	}
	saved, err := st.Baseline(baselines[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	changed := &model.Port{IfIndex: 7, Name: "1/0/7", ConfigSettings: map[string]string{"max_frame": "9216"}}
	e.OnState(&model.SwitchState{ID: 1, Name: "SW", Reachable: true, Ports: []*model.Port{changed}}, "slow")
	diffs := e.BaselineDiff(saved)
	if len(diffs) != 1 || diffs[0].ConfigField != "max_frame" || diffs[0].Before != "1522" || diffs[0].After != "9216" {
		t.Fatalf("unexpected baseline diff: %+v", diffs)
	}
	legacyPort := saved[0].Ports[0]
	legacyPort.MaxFrame = 1522
	legacyPort.ConfigSettings = nil
	diffs = e.BaselineDiff(saved)
	if len(diffs) != 1 || diffs[0].ConfigField != "max_frame" || diffs[0].Before != "1522" || diffs[0].After != "9216" {
		t.Fatalf("unexpected legacy baseline diff: %+v", diffs)
	}
	legacyPort.ConfigSettings = map[string]string{}
	if diffs = e.BaselineDiff(saved); len(diffs) != 0 {
		t.Fatalf("unknown baseline values yielded differences: %+v", diffs)
	}
}

func TestProductionPhaseRetriesAfterBaselineFailure(t *testing.T) {
	e, st := newTestEngine(t)
	if err := e.SetPhase("setup"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`CREATE TRIGGER reject_baseline BEFORE INSERT ON baselines BEGIN SELECT RAISE(ABORT, 'write failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPhase("show"); err == nil || e.Phase() != "setup" {
		t.Fatalf("failed baseline left phase at %s, err=%v", e.Phase(), err)
	}
	var phase string
	if err := st.DB.QueryRow(`SELECT v FROM meta WHERE k='op_phase'`).Scan(&phase); err != nil || phase != "setup" {
		t.Fatalf("stored phase: %q (%v)", phase, err)
	}
	if _, err := st.DB.Exec(`DROP TRIGGER reject_baseline`); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPhase("show"); err != nil {
		t.Fatal(err)
	}
	baselines, err := st.Baselines(e.ProfileID())
	if err != nil || e.Phase() != "show" || len(baselines) != 1 {
		t.Fatalf("retry: phase=%s, baselines=%+v (%v)", e.Phase(), baselines, err)
	}
}

func TestPortConfigChangeIsLogOnly(t *testing.T) {
	e, st := newTestEngine(t)
	ev := model.PortConfigChange(&model.Port{IfIndex: 7, Name: "1/0/7"}, "max_frame", "1522", "9216")
	ev.SwitchID = 1
	e.OnEvent(ev)
	events, err := st.Events(store.EventFilter{Category: model.CategoryConfig})
	if err != nil || len(events) != 1 || events[0].Severity != model.SevInfo || events[0].Acked {
		t.Fatalf("configuration log: %+v (%v)", events, err)
	}
	if alerts := e.Alerts(); len(alerts) != 0 {
		t.Fatalf("configuration change created alerts: %+v", alerts)
	}
}
