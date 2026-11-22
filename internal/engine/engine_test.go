package engine

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func newTestEngine(t *testing.T) (*Engine, *store.Store) {
	st, err := store.Open(filepath.Join(t.TempDir(), "e.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	pid, _ := st.EnsureDefaultProfile()
	sw := model.SwitchConfig{ProfileID: pid, Name: "SW", IP: "10.0.0.1", Enabled: true}
	st.SaveSwitch(&sw)
	return New(st, pid, nil), st
}

func codes(t *testing.T, st *store.Store) map[string]int {
	evs, err := st.Events(store.EventFilter{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]int{}
	for _, e := range evs {
		m[e.Code]++
	}
	return m
}

// 期待値は仕様書3.6の判定表から導出（実装出力のコピーではない）。
func TestRulesFromSpec(t *testing.T) {
	e, st := newTestEngine(t)
	s := &model.SwitchState{ID: 1, Name: "SW", Reachable: true, Ports: []*model.Port{
		{IfIndex: 1, Name: "1/0/1", Oper: "up", SpeedMbps: 1000, FCSDelta: 1}, // 物理エラー+1 → CRITICAL
		{IfIndex: 2, Name: "1/0/2", Oper: "up", SpeedMbps: 1000, PauseDelta: 5, AVRole: "ndi4"},
		{IfIndex: 3, Name: "1/0/3", Oper: "up", SpeedMbps: 1000, PauseDelta: 5, AVRole: "dante", EEE: "enable"},
		{IfIndex: 4, Name: "1/0/4", Oper: "up", SpeedMbps: 10000, SFP: &model.SFP{RxDBm: -10.5}}, // -10dBm未満
		{IfIndex: 5, Name: "1/0/5", Oper: "up", SpeedMbps: 1000, FCSDelta: 9, Excluded: true},    // 監視除外
		{IfIndex: 6, Name: "1/0/6", Oper: "up", SpeedMbps: 10000, SFP: &model.SFP{RxDBm: -9.9}},  // 基準内
	}, Health: model.Health{CPU5s: 80, MemFreePct: 15, TempMaxC: 40}}
	e.OnState(s, "medium")
	c := codes(t, st)
	want := map[string]int{"PHY_ERRORS": 1, "PAUSE_FRAMES": 0, "EEE_ON_AV_PORT": 0, "SFP_LOW_RX": 1, "CPU_HIGH": 1, "MEM_LOW": 1}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("%s = %d want %d (all=%v)", k, c[k], v, c)
		}
	}
	if c["TEMP_HIGH"] != 0 {
		t.Error("温度40℃で警告してはならない")
	}
	// 同じ条件が続いても再発報しない（立ち上がりのみ）
	e.OnState(s, "medium")
	if c2 := codes(t, st); c2["PHY_ERRORS"] != 1 {
		t.Errorf("再発報された: %d", c2["PHY_ERRORS"])
	}
	// 未確認ラッチの再発生は同じイベントを再利用する。
	s.Ports[0].FCSDelta = 0
	e.OnState(s, "medium")
	s.Ports[0].FCSDelta = 2
	e.OnState(s, "medium")
	if c3 := codes(t, st); c3["PHY_ERRORS"] != 1 {
		t.Errorf("未確認ラッチの重複発報: %d", c3["PHY_ERRORS"])
	}
}

func TestTopologyBlockingRequiresOperationalPort(t *testing.T) {
	for _, tt := range []struct {
		name, firstOper, firstSTP, secondOper, secondSTP string
		blocking                                         bool
	}{
		{"discarding on down source", "down", "discarding", "up", "forwarding", false},
		{"discarding on down peer", "up", "forwarding", "down", "discarding", false},
		{"discarding on up source", "up", "discarding", "up", "forwarding", true},
		{"discarding on up peer", "up", "forwarding", "up", "discarding", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := &Engine{
				order: []int64{1, 2},
				states: map[int64]*model.SwitchState{
					1: {ID: 1, SysName: "A", Ports: []*model.Port{{Name: "1", Oper: tt.firstOper, StpState: tt.firstSTP}},
						Neighbors: []model.Neighbor{{SysName: "B", LocalPort: "1", PortID: "2"}}},
					2: {ID: 2, SysName: "B", Ports: []*model.Port{{Name: "2", Oper: tt.secondOper, StpState: tt.secondSTP}},
						Neighbors: []model.Neighbor{{SysName: "A", LocalPort: "2", PortID: "1"}}},
				},
			}
			edges := e.Topology(nil).Edges
			if len(edges) != 1 || edges[0].Blocking != tt.blocking {
				t.Fatalf("edges = %+v, want one link with Blocking=%t", edges, tt.blocking)
			}
		})
	}
}

func TestAuditCrossSwitch(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name, fw string, dhcp bool, prio int) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, SysName: name, Reachable: true, FW: fw, LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: prio},
			DHCP:   model.DHCP{ServerOn: dhcp},
			IGMP:   model.IGMP{Vlans: []model.VlanIGMP{{VLAN: 10, Snooping: true}}}}
	}
	e.mu.Lock()
	e.states = map[int64]*model.SwitchState{1: mk(1, "A", "13.0.7.6", true, 32768), 2: mk(2, "B", "13.0.6.0", true, 32768)}
	e.order = []int64{1, 2}
	e.mu.Unlock()
	fs, fw := e.Audit()
	got := map[string]bool{}
	for _, f := range fs {
		got[f.Code] = true
	}
	for _, c := range []string{"FW_MISMATCH", "DHCP_DOUBLE", "STP_PRIORITY_DEFAULT", "QUERIER_ABSENT"} {
		if !got[c] {
			t.Errorf("%s が検出されない: %v", c, got)
		}
	}
	if len(fw) != 2 {
		t.Errorf("FW分類 %v", fw)
	}
	// 1台でも初回取得前なら横断監査は保留（監査3周目#5）
	e.mu.Lock()
	e.states[3] = &model.SwitchState{ID: 3, Name: "C"}
	e.order = append(e.order, 3)
	e.mu.Unlock()
	fs, _ = e.Audit()
	if len(fs) != 1 || fs[0].Code != "AUDIT_PENDING" {
		t.Errorf("初回取得前に横断監査が実行された: %+v", fs)
	}
}

func TestRecommendationsDoNotRaiseAlerts(t *testing.T) {
	e, st := newTestEngine(t)
	a := &model.SwitchState{ID: 1, Name: "A", SysName: "A", Reachable: true, FW: "1", LastPoll: time.Now(),
		Config: model.SwitchConf{StpVersion: "mstp", StpPriority: 32768, IsRoot: true},
		IGMP:   model.IGMP{Vlans: []model.VlanIGMP{{VLAN: 10, Snooping: true, LastVersion: 2, LastQuerier: "192.0.2.1"}}},
		Ports: []*model.Port{
			{IfIndex: 1, Name: "1/0/1", Oper: "up", AVRole: "dante", EEE: "enable", LPIDelta: 2, Pause: "enabledXmit", PauseDelta: 4, Trust: "trustDot1p", Duplex: "halfDuplex"},
			{IfIndex: 2, Name: "1/0/2", Oper: "up", AVRole: "ndi4", Pause: "disabled"},
			{IfIndex: 3, Name: "1/0/3", Oper: "up", AVRole: "qsys", MaxFrame: 9216},
			{IfIndex: 4, Name: "1/0/4", Oper: "up", AVRole: "uplink", MaxFrame: 9216, McastCtl: true, BcastCtl: true},
			{IfIndex: 5, Name: "1/0/5", Oper: "up", AVRole: "ndi4"},
			{IfIndex: 6, Name: "1/0/6", Oper: "up", AVRole: "dante", EEE: "enable", Excluded: true},
			{IfIndex: 7, Name: "1/0/7", Oper: "up", TooLongDelta: 1, FCSDelta: 1, SpeedMbps: 100, ExpectedSpeed: 1000, PoE: &model.PoE{Fault: "overload"},
				RawCounters: map[string]string{"dot3StatsFCSErrors": "112", "dot3StatsFrameTooLongs": "8"}},
		},
		Neighbors: []model.Neighbor{{SysName: "B", LocalPort: "1/0/4", PortID: "1/0/1", IsNetgear: true}},
	}
	b := &model.SwitchState{ID: 2, Name: "B", SysName: "B", Reachable: true, FW: "2", LastPoll: time.Now(),
		Config:    model.SwitchConf{StpVersion: "rstp", StpPriority: 32768},
		IGMP:      model.IGMP{Vlans: []model.VlanIGMP{{VLAN: 10, QuerierOper: "querier", LastVersion: 3, LastQuerier: "192.0.2.1"}}},
		Ports:     []*model.Port{{IfIndex: 1, Name: "1/0/1", Oper: "up", MaxFrame: 1522}},
		Neighbors: []model.Neighbor{{SysName: "A", LocalPort: "1/0/1", PortID: "1/0/4", IsNetgear: true}},
	}
	e.states, e.order = map[int64]*model.SwitchState{1: a, 2: b}, []int64{1, 2}
	for _, tier := range []string{"fast", "medium", "slow"} {
		e.evaluate(a, tier, DefaultSettings())
	}
	e.evaluateCross(DefaultSettings())
	got := codes(t, st)
	// DUPLEX (observed half duplex), IGMP_VERSION_MIX and SNOOP_MISMATCH are measured faults, so they still alert.
	for _, code := range []string{"PHY_ERRORS", "MTU_MISMATCH", "SPEED_BELOW_EXPECTED", "POE_PORT_FAULT", "DUPLEX", "IGMP_VERSION_MIX", "SNOOP_MISMATCH"} {
		if got[code] != 1 {
			t.Errorf("%s: %v", code, got)
		}
	}
	if len(got) != 7 {
		t.Errorf("unexpected events: %v", got)
	}
	var phy *Alert
	for _, alert := range e.Alerts() {
		if alert.Code == "PHY_ERRORS" {
			phy = &alert
		}
	}
	if phy == nil || phy.RawCounters["dot3StatsFCSErrors"] != "112" {
		t.Fatalf("raw counter missing: %+v", phy)
	}
	want := map[string][2]string{
		"EEE_ON_AV_PORT": {"enable", "disable"}, "FLOWCTRL_ON_AV_PORT": {"enabledXmit", "disable"},
		"DSCP_TRUST":          {"trustDot1p", "trustIpDscp"},
		"NDI4_NEEDS_FLOWCTRL": {"disabled", "enable"}, "QSYS_MTU": {"9216", "1522"},
		"BCAST_STORM_CTRL_UPLINK": {"enable", "disable"}, "MCAST_STORM_CTRL_UPLINK": {"enable", "disable"},
		"UPLINK_MTU_MISMATCH": {"9216", "1522"},
	}
	peerRows := 0
	for _, row := range PortRecommendations(e.States()) {
		if row.SwitchID == 2 && row.Port == "1/0/1" && row.Code == "UPLINK_MTU_MISMATCH" && row.Current == "1522" && row.Recommended == "9216" {
			peerRows++
			continue
		}
		values := [2]string{row.Current, row.Recommended}
		if row.SwitchID != 1 || row.Port == "1/0/5" || row.Port == "1/0/6" || values != want[row.Code] {
			t.Errorf("unexpected recommendation: %+v", row)
		}
		if (row.Code == "NDI4_NEEDS_FLOWCTRL" || row.Code == "UPLINK_MTU_MISMATCH") && row.Source != "NETGEAR ProAV Guide" {
			t.Errorf("source missing: %+v", row)
		}
		delete(want, row.Code)
	}
	if len(want) != 0 {
		t.Errorf("missing recommendations: %v", want)
	}
	if peerRows != 1 {
		t.Errorf("peer recommendations: %d", peerRows)
	}
}

func TestRestoreSkipsPastRecommendations(t *testing.T) {
	e, st := newTestEngine(t)
	for _, code := range []string{"EEE_ON_AV_PORT", "PHY_ERRORS"} {
		ev := model.Event{TS: time.Now(), Category: "ANOMALY", Severity: model.SevWarning, Code: code,
			SwitchID: 1, IfIndex: 1, Detail: map[string]any{"_profile_id": e.ProfileID()}}
		if inserted, err := st.InsertEvent(&ev); err != nil || !inserted {
			t.Fatalf("insert %s: %v (%v)", code, err, inserted)
		}
	}
	e.restoreAlerts()
	alerts := e.Alerts()
	if len(alerts) != 1 || alerts[0].Code != "PHY_ERRORS" {
		t.Fatalf("restored alerts: %+v", alerts)
	}
	if saved := codes(t, st); saved["EEE_ON_AV_PORT"] != 1 {
		t.Fatalf("past events were changed: %v", saved)
	}
}

func TestPortRecommendationsSkipMissingAndUnreachableData(t *testing.T) {
	states := []*model.SwitchState{nil,
		{ID: 1, Reachable: true, Ports: []*model.Port{nil}},
		{ID: 2, Reachable: false, Ports: []*model.Port{{AVRole: "dante", EEE: "enable"}}},
	}
	if rows := PortRecommendations(states); len(rows) != 0 {
		t.Fatalf("recommendations from unavailable data: %+v", rows)
	}
}

// リンクダウン中のポートは dot3PauseOperMode が disabled になるため、Flow Control の推奨判定に使わない
func TestPortRecommendationsIgnorePauseOnDownPort(t *testing.T) {
	states := []*model.SwitchState{{ID: 1, Reachable: true, Ports: []*model.Port{{Name: "1/0/3", Oper: "down", AVRole: "ndi4", Pause: "disabled"}}}}
	for _, row := range PortRecommendations(states) {
		if row.Code == "NDI4_NEEDS_FLOWCTRL" {
			t.Fatalf("pause judged on a down port: %+v", row)
		}
	}
}
