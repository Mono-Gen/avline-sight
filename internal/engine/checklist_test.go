package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

const checklistTestMAC = "00:11:22:33:44:55"

func TestChecklistSwitchWithoutCredentials(t *testing.T) {
	e, st := newTestEngine(t)
	sw := model.SwitchConfig{ProfileID: e.ProfileID(), Name: "Unconfigured", IP: "192.0.2.2", Enabled: true}
	if err := st.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	if s := e.State(sw.ID); s == nil || !s.NeedsCredentials {
		t.Fatalf("missing credentials not reflected in switch state: %+v", s)
	}
	summary := e.Overview(false).Switches
	if len(summary) != 2 || summary[1].ID != sw.ID || !summary[1].NeedsCredentials {
		t.Fatalf("missing credentials not reflected in overview: %+v", summary)
	}
	checklist, err := e.ShowChecklist(t.Context(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, item := range checklist.Items {
		if item.ID != "reach" && item.ID != "initial" {
			continue
		}
		if item.State != "review" {
			t.Errorf("%s = %s, want review", item.ID, item.State)
		}
		for _, finding := range item.Findings {
			if finding.Target.SwitchID == sw.ID {
				checked++
				if finding.State != "review" || finding.Reason != "SNMP community required" ||
					finding.Action != "Enter the SNMP community in Settings." || finding.Target.Destination != "settings" {
					t.Errorf("%s: unexpected credential finding: %+v", item.ID, finding)
				}
			}
		}
	}
	if checked != 2 {
		t.Fatalf("checked %d credential findings, want 2", checked)
	}
	sw.Community = "fixture"
	if err := st.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	if s := e.State(sw.ID); s.NeedsCredentials || !s.LastPoll.IsZero() {
		t.Fatalf("switch should wait for its first poll: %+v", s)
	}
}

func TestChecklistChassisAlertTargetsOnlyTheSwitch(t *testing.T) {
	e, _ := newTestEngine(t)
	e.states[1] = &model.SwitchState{ID: 1, Name: "Stage"}
	for _, code := range []string{"FAN_FAIL", "PSU_FAIL"} {
		e.active[code] = &Alert{SwitchID: 1, SwitchName: "Stage", IfIndex: 7, Port: "1/0/7", MAC: checklistTestMAC,
			Code: code, Severity: model.SevCritical, Active: true}
	}
	checklist, err := e.ShowChecklist(t.Context(), true, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range checklist.Items {
		if item.ID != "unacked" && item.ID != "active" {
			continue
		}
		if len(item.Findings) != 2 {
			t.Fatalf("%s: %v", item.ID, item.Findings)
		}
		for _, finding := range item.Findings {
			if want := (ChecklistTarget{SwitchID: 1, Label: "Stage"}); finding.Target != want {
				t.Errorf("%s: got %+v, want %+v", item.ID, finding.Target, want)
			}
		}
	}
}

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
				Health: model.Health{PoEBudgetW: 100, PoEUsedW: 40, TempMaxC: 32},
				// 実機は必ずVLAN 1を返す。IGMPの取得が済んだ健全なスイッチとして扱う
				IGMP: model.IGMP{SnoopingKnown: true, SnoopingOn: true, Vlans: []model.VlanIGMP{{VLAN: 1}}}}
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
			// 必須9項目（計画§16の8項目＋マルチキャスト設定）と任意7項目
			if !found || required != 9 || optional != 7 {
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

// マルチキャスト設定の5種類（クエリア不在・複数・Snooping不一致・IGMP版混在・全体OFF）を本番前チェックの必須項目に載せる。
func TestChecklistMulticastItem(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name string, on bool, vlans ...model.VlanIGMP) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, SysName: name, Reachable: true, FW: "13.0.7.6", LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: id == 1},
			IGMP:   model.IGMP{SnoopingKnown: true, SnoopingOn: on, Vlans: vlans}}
	}
	item := func(states ...*model.SwitchState) ChecklistItem {
		e.mu.Lock()
		e.states = map[int64]*model.SwitchState{}
		e.order = nil
		for _, s := range states {
			e.states[s.ID] = s
			e.order = append(e.order, s.ID)
		}
		e.mu.Unlock()
		checklist, err := e.ShowChecklist(t.Context(), true, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range checklist.Items {
			if it.ID == "multicast" {
				return it
			}
		}
		t.Fatal("multicast item missing")
		return ChecklistItem{}
	}
	ok := item(mk(1, "A", true, model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.0.2.11"}))
	if ok.State != checkPass || ok.Optional {
		t.Fatalf("a healthy VLAN: %+v", ok)
	}
	bad := item(
		mk(1, "A", true, model.VlanIGMP{VLAN: 11, Snooping: true, LastQuerier: "0.0.0.0"}, model.VlanIGMP{VLAN: 12, Snooping: true, QuerierOper: "querier", LastQuerier: "198.51.100.1"}),
		mk(2, "B", false, model.VlanIGMP{VLAN: 12, Snooping: false, QuerierOper: "querier", LastQuerier: "198.51.100.2"}, model.VlanIGMP{VLAN: 13, Snooping: true, QuerierOper: "querier", LastQuerier: "203.0.113.1"}))
	if bad.State != checkReview {
		t.Fatalf("problems not reported: %+v", bad)
	}
	var reasons []string
	for _, f := range bad.Findings {
		reasons = append(reasons, f.Target.Label+": "+f.Reason)
	}
	all := strings.Join(reasons, "\n")
	for _, want := range []string{
		"VLAN 11: VLAN 11: Snooping が有効ですがクエリアがいません。",
		"VLAN 12: VLAN 12: クエリアが複数います（198.51.100.1, 198.51.100.2）。",
		"VLAN 12: VLAN 12: スイッチによって Snooping の有効・無効が違います。",
		"B: スイッチ全体の Snooping が無効です（VLAN 13 の設定が効いていません）。",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	// 初回取得前は判定中
	pending := item(mk(1, "A", true), &model.SwitchState{ID: 2, Name: "B"})
	if pending.State != checkPending {
		t.Fatalf("before the first collection: %+v", pending)
	}
	// STP等(Slow)は取れたがIGMP(Medium)がまだ → 何も検査せずに合格しない
	slowOnly := item(mk(1, "A", true))
	if slowOnly.State != checkPending {
		t.Fatalf("passed before IGMP was collected: %+v", slowOnly)
	}
	// 判定できるスイッチが無い（全台到達不能・0台）→ 合格にも判定中にもせず要確認
	down := mk(1, "A", true, model.VlanIGMP{VLAN: 11, Snooping: true})
	down.Reachable = false
	if got := item(down); got.State != checkReview {
		t.Fatalf("all switches unreachable: %+v", got)
	}
	if got := item(); got.State != checkReview {
		t.Fatalf("no switches: %+v", got)
	}
	// LLDPでつながった2台がどちらも自分をクエリアと答える（アドレスは 0.0.0.0）→ クエリアが複数
	a := mk(1, "A", true, model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "0.0.0.0"})
	b := mk(2, "B", true, model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "0.0.0.0"})
	a.Neighbors = []model.Neighbor{{SysName: "B", IsNetgear: true}}
	linked := item(a, b)
	if linked.State != checkReview || !strings.Contains(linked.Findings[0].Reason, "クエリアが複数います（A, B）") {
		t.Fatalf("two linked queriers: %+v", linked)
	}
	// つながっていない2台（別ネットワークで同じVLAN番号）は対象外
	a.Neighbors = nil
	if got := item(a, b); got.State != checkPass {
		t.Fatalf("two unlinked switches with the same VLAN reported: %+v", got)
	}
	// 別々につながった2系統で、それぞれ2台がクエリア → 両方を、毎回同じ順で報告する
	c := mk(3, "C", true, model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "0.0.0.0"})
	d := mk(4, "D", true, model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "0.0.0.0"})
	a.Neighbors = []model.Neighbor{{SysName: "B", IsNetgear: true}}
	c.Neighbors = []model.Neighbor{{SysName: "D", IsNetgear: true}}
	for range 5 {
		two := item(a, b, c, d)
		if len(two.Findings) != 2 || !strings.Contains(two.Findings[0].Reason, "（A, B）") || !strings.Contains(two.Findings[1].Reason, "（C, D）") {
			t.Fatalf("two separate groups: %+v", two.Findings)
		}
	}
	// 全スイッチが初回の応答前 → 要確認ではなく判定中
	if got := item(&model.SwitchState{ID: 1, Name: "A"}, &model.SwitchState{ID: 2, Name: "B"}); got.State != checkPending {
		t.Fatalf("before any first poll: %+v", got)
	}
}
