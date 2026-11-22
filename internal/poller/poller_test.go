package poller

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/sim"
)

func TestSnapshotPreservesRawCounterWithoutSharingWorkerMap(t *testing.T) {
	p := &model.Port{IfIndex: 1}
	cols := map[string]gosnmp.SnmpPDU{
		mib.CosQueueIntfHCTotalDrop: {Type: gosnmp.Counter64, Value: ^uint64(0)},
	}
	p.RawCounters = captureCounters(nil, cols, cosCounters)
	w := worker{st: &model.SwitchState{Ports: []*model.Port{p}}}
	snapshot := w.snapshot()
	p.RawCounters = captureCounters(p.RawCounters, nil, cosCounters)
	const name = "agentCosQueueIntfCounterHCTotalDropPackets"
	if got := snapshot.Ports[0].RawCounters[name]; got != "18446744073709551615" {
		t.Fatalf("snapshot raw counter: %q", got)
	}
	if _, ok := p.RawCounters[name]; ok {
		t.Fatal("missing data left a stale raw counter")
	}
}

type capSink struct {
	mu     sync.Mutex
	states map[int64]*model.SwitchState
	tiers  map[string]int
	events []model.Event
}

func (c *capSink) OnState(st *model.SwitchState, tier string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states[st.ID] = st
	c.tiers[tier]++
}
func (c *capSink) OnEvent(ev model.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, ev)
}

func (c *capSink) has(code string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.events {
		if e.Code == code {
			return true
		}
	}
	return false
}

// シミュレータに対して全Tierが回り、主要データが正規化されることを確認する結合テスト。
func TestPollerAgainstSim(t *testing.T) {
	lab := sim.NewLab(2, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := lab.Start(ctx, sim.Config{BaseSNMPPort: 27100}); err != nil {
		t.Fatal(err)
	}
	sink := &capSink{states: map[int64]*model.SwitchState{}, tiers: map[string]int{}}
	iv := Intervals{Fast: 300 * time.Millisecond, Medium: 600 * time.Millisecond, Slow: 900 * time.Millisecond, Timeout: time.Second}
	m := NewManager(iv, sink, nil)
	m.Sync(ctx, []model.SwitchConfig{
		{ID: 1, Name: "core", IP: "127.0.0.1", SNMPPort: 27100, Community: "public", Enabled: true},
		{ID: 2, Name: "edge", IP: "127.0.0.1", SNMPPort: 27101, Community: "public", Enabled: true},
		{ID: 3, Name: "dead", IP: "127.0.0.1", SNMPPort: 27199, Community: "public", Enabled: true},
	})
	defer m.StopAll()
	time.Sleep(3 * time.Second)

	sink.mu.Lock()
	core := sink.states[1]
	dead := sink.states[3]
	sink.mu.Unlock()
	if core == nil || !core.Reachable {
		t.Fatalf("coreに到達できていない: %+v", core)
	}
	if core.SysName != "AV-SW-01" || core.FW != "13.0.7.6" {
		t.Errorf("system: %s %s", core.SysName, core.FW)
	}
	if offset := core.ClockOffsetSeconds; offset == nil || math.Abs(*offset) > 1 {
		t.Errorf("Slow clock sample: %v", offset)
	}
	if len(core.Ports) != 30 {
		t.Errorf("ポート数=%d want 30", len(core.Ports))
	}
	var withDev, sfp, poe int
	var rx float64
	for _, p := range core.Ports {
		if len(p.Devices) > 0 {
			withDev++
		}
		if p.Type == "sfp" {
			sfp++
		}
		if p.PoE != nil && p.PoE.PowerW > 0 {
			poe++
		}
		rx += p.RxBps
	}
	if withDev < 10 || sfp != 4 || poe == 0 || rx <= 0 {
		t.Errorf("機器=%d SFP=%d PoE給電=%d rx=%.0f", withDev, sfp, poe, rx)
	}
	if len(core.IGMP.Vlans) != 5 || len(core.IGMP.Groups) == 0 {
		t.Errorf("IGMP vlans=%d groups=%d", len(core.IGMP.Vlans), len(core.IGMP.Groups))
	}
	if !core.DHCP.ServerOn || len(core.DHCP.Leases) == 0 {
		t.Errorf("DHCP: %+v", core.DHCP.ServerOn)
	}
	if core.Health.TempMaxC <= 0 || len(core.Health.Fans) == 0 || core.Health.PoEBudgetW == 0 {
		t.Errorf("health: %+v", core.Health)
	}
	if !core.Config.IsRoot || core.Config.StpVersion != "rstp" {
		t.Errorf("STP: %+v", core.Config)
	}
	ng := 0
	for _, n := range core.Neighbors {
		if n.IsNetgear {
			ng++
		}
	}
	if ng != 1 {
		t.Errorf("NETGEAR隣接=%d want 1", ng)
	}
	if dead == nil || dead.Reachable {
		t.Error("応答しないスイッチがReachable扱い")
	}
	if !sink.has("SWITCH_UNREACHABLE") || !sink.has("SWITCH_LOG") {
		t.Error("到達不能イベントまたは内部ログが生成されていない")
	}
	if sink.has(model.CodePortConfigChanged) {
		t.Fatal("configuration change recorded before settings changed")
	}
	if err := lab.Inject(0, "av-landmines"); err != nil {
		t.Fatal(err)
	}

	// 障害注入: 再起動とリンク速度低下を検知できる
	if err := lab.Inject(1, "chassis"); err != nil {
		t.Fatal(err)
	}
	if err := lab.Inject(1, "link-flap"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(6 * time.Second)
	for _, code := range []string{"SWITCH_REBOOT", "LINK_DOWN", "LINK_UP"} {
		if !sink.has(code) {
			t.Errorf("%s が検知されていない", code)
		}
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	var changes []model.Event
	for _, event := range sink.events {
		if event.Code == model.CodePortConfigChanged {
			changes = append(changes, event)
		}
	}
	if len(changes) != 1 || changes[0].SwitchID != 1 || changes[0].Severity != model.SevInfo ||
		changes[0].Detail["field"] != "eee" || changes[0].Detail["before"] != "disable" || changes[0].Detail["after"] != "enable" {
		t.Fatalf("want one EEE change and no operational changes, got %+v", changes)
	}
}
