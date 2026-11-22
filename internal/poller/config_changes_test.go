package poller

import (
	"context"
	"errors"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func TestPortConfigChangesAcrossPolls(t *testing.T) {
	sink := &capSink{states: map[int64]*model.SwitchState{}, tiers: map[string]int{}}
	w := newWorker(model.SwitchConfig{ID: 42, Name: "Switch"}, Intervals{}, sink, nil, nil)
	port := &model.Port{IfIndex: 7, Name: "1/0/7"}
	w.st.Ports = []*model.Port{port}
	ctx := context.Background()

	for _, tc := range []struct {
		name      string
		frame     int
		storm     int64
		err       error
		wantFrom  string
		wantTo    string
		wantField string
	}{
		{name: "first poll", frame: 1522, storm: 2},
		{name: "unchanged", frame: 1522, storm: 2},
		{name: "max frame changes", frame: 9216, storm: 2, wantField: "max_frame", wantFrom: "1522", wantTo: "9216"},
		{name: "storm control changes", frame: 9216, storm: 1, wantField: "bcast_storm_ctl", wantFrom: "disabled", wantTo: "enabled"},
		{name: "current values unknown"},
		{name: "previous values unknown", frame: 1522, storm: 2},
		{name: "unreachable", frame: 9216, storm: 1, err: errors.New("timeout")},
		{name: "first poll after recovery", frame: 9216, storm: 1},
		{name: "change after recovery", frame: 1522, storm: 1, wantField: "max_frame", wantFrom: "9216", wantTo: "1522"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sink.events = nil
			w.poll(ctx, "slow", func() error {
				port.Oper = "up"
				port.SpeedMbps += 100
				port.ErrDelta++
				w.recordPositiveSetting(port, "max_frame", tc.frame)
				w.recordStormSetting(port, "bcast_storm_ctl", tc.storm)
				return tc.err
			})
			var changes []model.Event
			for _, event := range sink.events {
				if event.Code == model.CodePortConfigChanged {
					changes = append(changes, event)
				}
			}
			if tc.wantField == "" {
				if len(changes) != 0 {
					t.Fatalf("unexpected configuration changes: %+v", changes)
				}
				return
			}
			if len(changes) != 1 {
				t.Fatalf("want one configuration change, got %+v", changes)
			}
			ev := changes[0]
			if ev.Category != model.CategoryConfig || ev.Severity != model.SevInfo || ev.SwitchID != 42 || ev.IfIndex != 7 ||
				ev.Detail["field"] != tc.wantField || ev.Detail["before"] != tc.wantFrom || ev.Detail["after"] != tc.wantTo {
				t.Fatalf("unexpected change: %+v", ev)
			}
		})
	}
}
