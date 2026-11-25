package poller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/sim"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

// 取得に失敗したらUDPソケットを作り直し、スイッチが応答を再開すれば再起動なしで復帰する。
// 実機では監視PCのUSB LANアダプタが固まって戻ったあと、古いソケットのままタイムアウトし続けた（2026-11-25）。
func TestWorkerRedialsAfterFailureAndRecovers(t *testing.T) {
	var dials atomic.Int32
	original := dialSNMP
	dialSNMP = func(ctx context.Context, host string, port uint16, community string, timeout time.Duration, retries int) (*sx.Client, error) {
		dials.Add(1)
		return original(ctx, host, port, community, timeout, retries)
	}
	t.Cleanup(func() { dialSNMP = original })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := &capSink{states: map[int64]*model.SwitchState{}, tiers: map[string]int{}}
	iv := Intervals{Fast: 300 * time.Millisecond, Medium: 600 * time.Millisecond, Slow: 900 * time.Millisecond, Timeout: 200 * time.Millisecond}
	m := NewManager(iv, sink, nil)
	m.Sync(ctx, []model.SwitchConfig{{ID: 1, Name: "core", IP: "127.0.0.1", SNMPPort: 27198, Community: "public", Enabled: true}})
	defer m.StopAll()

	// 最初の取得は応答なしで失敗する → 作り直しで2回目のダイヤルが起きる
	deadline := time.Now().Add(3 * time.Second)
	for dials.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := dials.Load(); got < 2 {
		t.Fatalf("dials after a failed poll = %d, want at least 2 (initial + redial)", got)
	}
	if !sink.has("SWITCH_UNREACHABLE") {
		t.Fatal("the failed poll did not raise SWITCH_UNREACHABLE")
	}

	// スイッチが応答を始めたら、次の再試行（5秒後）で復帰する
	lab := sim.NewLab(1, 1)
	if err := lab.Start(ctx, sim.Config{BaseSNMPPort: 27198}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		sink.mu.Lock()
		st := sink.states[1]
		reachable := st != nil && st.Reachable
		sink.mu.Unlock()
		if reachable {
			if !sink.has("SWITCH_RECOVERED") {
				t.Fatal("recovered without SWITCH_RECOVERED")
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("the worker did not recover after the switch started answering")
}
