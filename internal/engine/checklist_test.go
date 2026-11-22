package engine

import (
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

const checklistTestMAC = "00:11:22:33:44:55"

type checklistFixture struct {
	e         *Engine
	sw        *model.SwitchState
	port      *model.Port
	trapReady bool
}

func TestShowChecklistStates(t *testing.T) {
	cases := []struct {
		name, id, state string
		change          func(*testing.T, *checklistFixture)
	}{
		{"unreachable", "reach", "review", func(_ *testing.T, f *checklistFixture) { f.sw.Reachable = false }},
		{"no first response", "reach", "pending", func(_ *testing.T, f *checklistFixture) { f.sw.LastPoll = time.Time{} }},
		{"trap before startup", "trap", "review", func(_ *testing.T, f *checklistFixture) { f.e.lastTraps[1] = f.e.startedAt.Add(-time.Second) }},
		{"missing offset", "clock", "review", func(_ *testing.T, f *checklistFixture) { f.sw.ClockOffsetSeconds = nil }},
		{"outside clock tolerance", "clock", "review", func(_ *testing.T, f *checklistFixture) { offset := -60.1; f.sw.ClockOffsetSeconds = &offset }},
		{"never observed device", "devices", "review", func(_ *testing.T, f *checklistFixture) { f.port.Devices = nil }},
		{"recovered unacknowledged", "unacked", "review", func(_ *testing.T, f *checklistFixture) {
			f.e.active["latched"] = &Alert{SwitchID: 1, Severity: model.SevWarning, Code: "PHY_ERRORS"}
		}},
		{"active and acknowledged", "active", "review", func(_ *testing.T, f *checklistFixture) {
			f.e.active["active"] = &Alert{SwitchID: 1, Severity: model.SevWarning, Code: "PHY_ERRORS", Active: true, Acked: true}
		}},
		{"slow poll pending", "initial", "pending", func(_ *testing.T, f *checklistFixture) { f.sw.Config.StpVersion = "" }},
		{"first poll failed", "initial", "review", func(_ *testing.T, f *checklistFixture) { f.sw.Config.StpVersion = ""; f.sw.Reachable = false }},
		{"trap listener stopped", "host", "review", func(_ *testing.T, f *checklistFixture) { f.trapReady = false }},
		{"sleep inhibition not verified", "host", "review", func(_ *testing.T, f *checklistFixture) { f.e.sleepInhibited = false }},
		{"exclusion before port discovery", "excluded", "review", func(_ *testing.T, f *checklistFixture) { f.e.portCfg[1][9] = store.PortSetting{Excluded: true} }},
		{"ping for previous IP", "ping", "review", func(_ *testing.T, f *checklistFixture) {
			f.e.pingResults[checklistTestMAC] = DevicePing{IP: "192.0.2.20", OK: true, TS: time.Now()}
		}},
		{"failed ping", "ping", "review", func(_ *testing.T, f *checklistFixture) {
			f.e.pingResults[checklistTestMAC] = DevicePing{IP: "192.0.2.10", OK: false, TS: time.Now()}
		}},
		{"expired ping", "ping", "review", func(_ *testing.T, f *checklistFixture) {
			f.e.pingResults[checklistTestMAC] = DevicePing{IP: "192.0.2.10", OK: true, TS: time.Now().Add(-3 * time.Minute)}
		}},
		{"wrong planned IP", "ping", "review", func(t *testing.T, f *checklistFixture) { saveChecklistAlias(t, f.e, "192.0.2.20") }},
		{"drop within window", "recent", "review", func(t *testing.T, f *checklistFixture) {
			p := &model.Port{IfIndex: 7, DiscardDelta: 1}
			if err := f.e.st.InsertPortMetrics(time.Now().Add(-time.Minute), 1, []*model.Port{p}, false); err != nil {
				t.Fatal(err)
			}
		}},
		{"old error outside window", "recent", "pass", func(t *testing.T, f *checklistFixture) {
			insertChecklistEvent(t, f.e, model.Event{TS: time.Now().Add(-6 * time.Minute), Code: "LINK_FLAP", Category: "CONNECT", Severity: model.SevWarning, SwitchID: 1, IfIndex: 7})
		}},
		{"optical margin below 3 dB", "optics", "review", func(_ *testing.T, f *checklistFixture) { f.port.SFP.RxDBm = -8 }},
		{"low uplink speed", "uplink", "review", func(_ *testing.T, f *checklistFixture) { f.port.SpeedMbps = 100 }},
		{"unexpected STP state", "uplink", "review", func(_ *testing.T, f *checklistFixture) { f.port.StpState = "discarding" }},
		{"PoE headroom", "power", "review", func(_ *testing.T, f *checklistFixture) { f.sw.Health.PoEUsedW = 71 }},
		{"temperature headroom", "power", "review", func(_ *testing.T, f *checklistFixture) { f.sw.Health.TempMaxC = 61 }},
		{"missing redundant power", "power", "review", func(_ *testing.T, f *checklistFixture) {
			f.sw.Health.PSUs = []model.Item{{State: "operational"}, {State: "notpresent"}}
		}},
		{"recent config", "config", "review", func(t *testing.T, f *checklistFixture) {
			ev := model.PortConfigChange(f.port, "max_frame", "1522", "9216")
			ev.SwitchID, ev.TS = 1, time.Now().Add(-10*time.Minute)
			insertChecklistEvent(t, f.e, ev)
		}},
		{"all checks pass", "", "pass", func(_ *testing.T, _ *checklistFixture) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newTestEngine(t)
			now := time.Now()
			offset := 60.0
			port := &model.Port{IfIndex: 7, Name: "1/0/7", Oper: "up", AVRole: "uplink", SpeedMbps: 1000, ExpectedSpeed: 1000,
				SFP: &model.SFP{RxDBm: -7}, StpState: "forwarding",
				Devices: []model.Device{{MAC: checklistTestMAC, IP: "192.0.2.10", Source: "fdb"}}}
			sw := &model.SwitchState{ID: 1, Name: "SW", LastPoll: now, Reachable: true, ClockOffsetSeconds: &offset,
				Config: model.SwitchConf{StpVersion: "rstp"}, Ports: []*model.Port{port},
				Health: model.Health{PoEBudgetW: 100, PoEUsedW: 40, TempMaxC: 32}}
			e.states[1], e.lastTraps[1], e.sleepInhibited = sw, now, true
			e.pingResults[checklistTestMAC] = DevicePing{IP: "192.0.2.10", OK: true, TS: now}
			saveChecklistAlias(t, e, "192.0.2.10")
			fixture := &checklistFixture{e: e, sw: sw, port: port, trapReady: true}
			tc.change(t, fixture)
			checklist, err := e.ShowChecklist(t.Context(), fixture.trapReady, true)
			if err != nil {
				t.Fatal(err)
			}
			required, optional := 0, 0
			found := false
			for _, item := range checklist.Items {
				if item.Optional {
					optional++
				} else {
					required++
				}
				if item.ID == tc.id || tc.id == "" {
					found = true
					if item.State != tc.state || item.Reason == "" || item.Action == "" {
						t.Errorf("%s: wanted %s with a reason and action, got %+v", item.ID, tc.state, item)
					}
					if tc.state == "review" && len(item.Findings) == 0 {
						t.Errorf("%s: missing findings", item.ID)
					}
				}
				if tc.name == "active and acknowledged" && item.ID == "unacked" && item.State != "pass" {
					t.Error("acknowledged alert counted as unacknowledged")
				}
				if tc.name == "recovered unacknowledged" && item.ID == "active" && item.State != "pass" {
					t.Error("recovered alert counted as active")
				}
			}
			if !found || required != 8 || optional != 7 {
				t.Fatalf("found %v, required %d, optional %d", found, required, optional)
			}
		})
	}
}

func saveChecklistAlias(t *testing.T, e *Engine, ip string) {
	t.Helper()
	if err := e.st.SaveAlias(e.profileID, store.Alias{MatchType: "mac", MatchKey: checklistTestMAC, Label: "Device", PlannedIP: ip}); err != nil {
		t.Fatal(err)
	}
}

func insertChecklistEvent(t *testing.T, e *Engine, event model.Event) {
	t.Helper()
	if inserted, err := e.st.InsertEvent(&event); !inserted || err != nil {
		t.Fatalf("insert event=%v, %v", inserted, err)
	}
}
