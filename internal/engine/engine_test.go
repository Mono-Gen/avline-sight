package engine

import (
	"path/filepath"
	"testing"

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
		{IfIndex: 1, Name: "1/0/1", Oper: "up", SpeedMbps: 1000, FCSDelta: 1},                                   // 物理エラー+1 → CRITICAL
		{IfIndex: 2, Name: "1/0/2", Oper: "up", SpeedMbps: 1000, PauseDelta: 5, AVRole: "ndi4"},                 // NDI4はPAUSE許容
		{IfIndex: 3, Name: "1/0/3", Oper: "up", SpeedMbps: 1000, PauseDelta: 5, AVRole: "dante", EEE: "enable"}, // PAUSE+EEE警告
		{IfIndex: 4, Name: "1/0/4", Oper: "up", SpeedMbps: 10000, SFP: &model.SFP{RxDBm: -10.5}},                // -10dBm未満
		{IfIndex: 5, Name: "1/0/5", Oper: "up", SpeedMbps: 1000, FCSDelta: 9, Excluded: true},                   // 監視除外
		{IfIndex: 6, Name: "1/0/6", Oper: "up", SpeedMbps: 10000, SFP: &model.SFP{RxDBm: -9.9}},                 // 基準内
	}, Health: model.Health{CPU5s: 80, MemFreePct: 15, TempMaxC: 40}}
	e.OnState(s, "medium")
	c := codes(t, st)
	want := map[string]int{"PHY_ERRORS": 1, "PAUSE_FRAMES": 1, "EEE_ON_AV_PORT": 1, "SFP_LOW_RX": 1, "CPU_HIGH": 1, "MEM_LOW": 1}
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
	// 解消後に再度成立したら再発報
	s.Ports[0].FCSDelta = 0
	e.OnState(s, "medium")
	s.Ports[0].FCSDelta = 2
	e.OnState(s, "medium")
	if c3 := codes(t, st); c3["PHY_ERRORS"] != 2 {
		t.Errorf("解消後の再発報なし: %d", c3["PHY_ERRORS"])
	}
}

func TestAuditCrossSwitch(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name, fw string, dhcp bool, prio int) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, SysName: name, Reachable: true, FW: fw,
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
}
