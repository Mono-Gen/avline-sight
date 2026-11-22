package poller

import (
	"context"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/sim"
)

func TestTrapRequestDebounceAndFDBCoalescing(t *testing.T) {
	now := time.Now()
	q := requests{wake: make(chan struct{}, 1)}
	q.addTrap(model.Event{Code: "UNKNOWN_TRAP"})
	if fast, medium, slow := q.takeTrap(now); !fast || medium || slow {
		t.Fatal("通常TrapはFastのみ")
	}
	for range 100 {
		q.addTrap(model.Event{Code: "linkDown", IfIndex: 7})
		q.addTrap(model.Event{Code: "authenticationFailure"})
	}
	q.addTrap(model.Event{Code: "boxsTemperatureChange"})
	if fast, medium, slow := q.takeTrap(now.Add(4999 * time.Millisecond)); fast || medium || slow {
		t.Fatal("5秒未満で再取得")
	}
	if fast, medium, slow := q.takeTrap(now.Add(5 * time.Second)); !fast || !medium || !slow {
		t.Fatal("必要なMediumとSlowを含めてまとめる")
	}
	if fast, medium, slow := q.takeTrap(now.Add(10 * time.Second)); fast || medium || slow {
		t.Fatal("追加取得が積み上がった")
	}
	if ports := q.takeLinkSignals(); len(ports[7].downs) != 1 || len(ports) != 1 {
		t.Fatalf("リンク断の保留: %v", ports)
	}
	q.addFDB(now.Add(10 * time.Second))
	q.addFDB(now.Add(12 * time.Second))
	if q.takeFDB(now.Add(9999*time.Millisecond), false) || !q.takeFDB(now.Add(10*time.Second), false) || q.takeFDB(now.Add(12*time.Second), false) {
		t.Fatal("FDBの期限または集約が不正")
	}
	q.addFDB(now.Add(10 * time.Second))
	if q.takeFDB(now.Add(8*time.Second), true) || q.takeFDB(now.Add(10*time.Second), false) {
		t.Fatal("既存Slow取得とFDB単独取得が重複")
	}
	q.addFDB(now.Add(10 * time.Second))
	q.addFDB(now)
	if !q.takeFDB(now, false) {
		t.Fatal("リンク断による即時取得を優先しない")
	}
}

func TestPhaseIntervals(t *testing.T) {
	for _, phase := range []string{"setup", "show", "rehearsal"} {
		want := []time.Duration{3 * time.Second, 12 * time.Second, 45 * time.Second}
		if phase == "setup" {
			want = []time.Duration{9 * time.Second, 36 * time.Second, 90 * time.Second}
		}
		for i, tier := range []string{"fast", "medium", "slow"} {
			base := []time.Duration{3 * time.Second, 12 * time.Second, 45 * time.Second}[i]
			if got := intervalForPhase(base, tier, phase); got != want[i] {
				t.Fatalf("%s/%s=%s want %s", phase, tier, got, want[i])
			}
		}
	}
}

func TestTrapStartsFastAndSlowOnExistingWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lab := sim.NewLab(1, 5)
	if err := lab.Start(ctx, sim.Config{BaseSNMPPort: 27220}); err != nil {
		t.Fatal(err)
	}
	sink := &capSink{states: map[int64]*model.SwitchState{}, tiers: map[string]int{}}
	m := NewManager(Intervals{Fast: time.Hour, Medium: time.Hour, Slow: time.Hour, Timeout: time.Second}, sink, nil)
	m.Sync(ctx, []model.SwitchConfig{{ID: 1, Name: "SW", IP: "127.0.0.1", SNMPPort: 27220, Community: "public", Enabled: true}})
	defer m.StopAll()
	waitFor := func(f func() bool) {
		t.Helper()
		deadline := time.After(4 * time.Second)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for !f() {
			select {
			case <-deadline:
				t.Fatal("収集が開始されない")
			case <-tick.C:
			}
		}
	}
	counts := func(fast, slow int) bool {
		sink.mu.Lock()
		defer sink.mu.Unlock()
		return sink.tiers["fast"] >= fast && sink.tiers["slow"] >= slow
	}
	waitFor(func() bool { return counts(1, 1) })
	sink.mu.Lock()
	var port int
	for _, p := range sink.states[1].Ports {
		if p.Oper == "up" {
			port = p.IfIndex
			break
		}
	}
	sink.mu.Unlock()
	if port == 0 {
		t.Fatal("リンクアップのポートなし")
	}
	m.TriggerTrap(model.Event{SwitchID: 1, Code: "linkDown", IfIndex: port})
	waitFor(func() bool { return counts(2, 2) })
	if !sink.has("LINK_FLAP") || sink.has("LINK_DOWN") {
		t.Fatal("Trap後もリンクアップならフラップのみ")
	}
}

func TestChassisTrapStartsMediumOnExistingWorker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const port = 27221
	lab := sim.NewLab(1, 5)
	if err := lab.Start(ctx, sim.Config{BaseSNMPPort: port}); err != nil {
		t.Fatal(err)
	}
	sink := &capSink{states: map[int64]*model.SwitchState{}, tiers: map[string]int{}}
	m := NewManager(Intervals{Fast: time.Hour, Medium: time.Hour, Slow: time.Hour, Timeout: time.Second}, sink, nil)
	m.Sync(ctx, []model.SwitchConfig{{ID: 1, Name: "SW", IP: "127.0.0.1", SNMPPort: port, Community: "public", Enabled: true}})
	defer m.StopAll()

	waitMedium := func(want int) {
		t.Helper()
		deadline := time.After(4 * time.Second)
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for {
			sink.mu.Lock()
			got := sink.tiers["medium"]
			sink.mu.Unlock()
			if got >= want {
				return
			}
			select {
			case <-deadline:
				t.Fatalf("Medium取得回数 = %d, want %d", got, want)
			case <-tick.C:
			}
		}
	}
	waitMedium(1)
	m.TriggerTrap(model.Event{SwitchID: 1, Code: "boxsFanStateChange"})
	waitMedium(2)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.tiers["fast"] != 2 || sink.tiers["slow"] != 1 {
		t.Fatalf("筐体Trapの取得階層: %+v", sink.tiers)
	}
}

func TestLinkFlapFromIfLastChange(t *testing.T) {
	w := worker{ifLastChange: map[int]uint64{8: 100}}
	p := &model.Port{IfIndex: 8, Name: "1/0/8", Oper: "up"}
	w.observeLinkFlap(p, "up", linkSignals{}, map[int]uint64{8: 200})
	if len(w.events) != 1 || w.events[0].Code != "LINK_FLAP" || w.events[0].Severity != model.SevWarning || w.events[0].Detail["source"] != "ifLastChange" {
		t.Fatalf("%+v", w.events)
	}
	w.observeLinkFlap(p, "up", linkSignals{}, map[int]uint64{8: 100})
	w.observeLinkFlap(p, "up", linkSignals{}, map[int]uint64{8: 30})
	w.observeLinkFlap(p, "up", linkSignals{}, nil)
	w.observeLinkFlap(p, "down", linkSignals{downs: []*uint64{nil}}, map[int]uint64{8: 200})
	p.Oper = "down"
	w.observeLinkFlap(p, "up", linkSignals{downs: []*uint64{nil}}, map[int]uint64{8: 200})
	if len(w.events) != 1 {
		t.Fatalf("変化なし・巻き戻り・不明・観測した断をフラップ扱い: %+v", w.events)
	}
}

func TestLinkFlapCountsDistinctTrapCyclesBetweenPolls(t *testing.T) {
	w := worker{ifLastChange: map[int]uint64{8: 100}}
	p := &model.Port{IfIndex: 8, Name: "1/0/8", Oper: "up"}
	for _, code := range []string{"linkDown", "linkDown", "linkUp", "linkDown", "linkUp"} {
		w.requests.addTrap(model.Event{Code: code, IfIndex: 8})
	}
	w.observeLinkFlap(p, "up", w.requests.takeLinkSignals()[8], map[int]uint64{8: 200})
	if len(w.events) != 1 || w.events[0].Detail["count"] != 2 {
		t.Fatalf("断復帰2回と重複通知1回: %+v", w.events)
	}
}

func TestLinkFlapDoesNotCountDelayedTrapAgain(t *testing.T) {
	w := worker{ifLastChange: map[int]uint64{8: 100}}
	p := &model.Port{IfIndex: 8, Name: "1/0/8", Oper: "up"}
	// Fastが保留を取り出した後、同じ取得中に断復帰が発生する。
	inFlight := w.requests.takeLinkSignals()
	late := uint64(150)
	w.requests.addTrap(model.Event{Code: "linkDown", IfIndex: 8, TrapUptime: &late})
	w.observeLinkFlap(p, "up", inFlight[8], map[int]uint64{8: 200})
	w.ifLastChange[8] = 200
	w.observeLinkFlap(p, "up", w.requests.takeLinkSignals()[8], map[int]uint64{8: 200})
	if len(w.events) != 1 {
		t.Fatalf("同じ断を再計上: %+v", w.events)
	}
	fresh := uint64(250)
	w.requests.addTrap(model.Event{Code: "linkDown", IfIndex: 8, TrapUptime: &fresh})
	w.observeLinkFlap(p, "up", w.requests.takeLinkSignals()[8], map[int]uint64{8: 275})
	if len(w.events) != 2 || w.events[1].Detail["count"] != 1 {
		t.Fatalf("別の断を取りこぼした: %+v", w.events)
	}
}
