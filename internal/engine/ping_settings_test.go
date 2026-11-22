package engine

import (
	"context"
	"testing"
	"time"
)

func TestPingIntervalBoundsAndPersistence(t *testing.T) {
	e, st := newTestEngine(t)
	if got := e.Settings(); got.SetupPingIntervalSec != 60 || got.RehearsalPingIntervalSec != 60 {
		t.Fatal(got)
	}
	for _, seconds := range []int{-1, 0, 29, 30, 60, 3600, 3601} {
		for _, rehearsal := range []bool{false, true} {
			s := DefaultSettings()
			if rehearsal {
				s.RehearsalPingIntervalSec = seconds
			} else {
				s.SetupPingIntervalSec = seconds
			}
			err := e.SaveSettings(s)
			valid := seconds >= 30 && seconds <= 3600
			if (err == nil) != valid {
				t.Fatalf("seconds=%d rehearsal=%v: %v", seconds, rehearsal, err)
			}
		}
	}
	s := DefaultSettings()
	s.SetupPingIntervalSec = 30
	s.RehearsalPingIntervalSec = 120
	if err := e.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	e = New(st, e.ProfileID(), nil)
	for _, tc := range []struct {
		phase string
		want  time.Duration
	}{{"setup", 30 * time.Second}, {"rehearsal", 120 * time.Second}} {
		if err := e.SetPhase(tc.phase); err != nil {
			t.Fatal(err)
		}
		if got := e.pingInterval(); got != tc.want {
			t.Fatalf("%s: %s", tc.phase, got)
		}
	}
	if err := st.SetProfileJSON(e.ProfileID(), "settings_json", map[string]any{"alert_sound": true}); err != nil {
		t.Fatal(err)
	}
	if got := New(st, e.ProfileID(), nil).Settings(); got.SetupPingIntervalSec != 60 || got.RehearsalPingIntervalSec != 60 {
		t.Fatal(got)
	}
}

func TestCancelledSleepPrevention(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := preventSleep(ctx); err != nil {
		t.Fatal(err)
	}
}
