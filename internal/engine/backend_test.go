package engine

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/ping"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

type recordingHub struct {
	mu    sync.Mutex
	types []string
}

func (h *recordingHub) Broadcast(kind string, _ any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.types = append(h.types, kind)
}
func (h *recordingHub) BroadcastSwitch(_ int64, _ *model.SwitchState) {}
func (h *recordingHub) has(kind string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, k := range h.types {
		if k == kind {
			return true
		}
	}
	return false
}

func alertEvent(code string, ix int) model.Event {
	return model.Event{SwitchID: 1, SwitchName: "SW", IfIndex: ix, Category: "ANOMALY", Severity: model.SevWarning, Code: code, Title: "検証"}
}

func TestAlertLatchLifecycle(t *testing.T) {
	e, st := newTestEngine(t)
	h := &recordingHub{}
	e.hub = h
	ev := alertEvent("TEST", 1)
	key := "1|WARNING|TEST|1|fast"
	e.raise(map[string]bool{}, key, ev)
	a := e.Alerts()
	if len(a) != 1 || !a[0].Active || a[0].EventID == 0 || a[0].State != "active" {
		t.Fatalf("発生: %+v", a)
	}
	id := a[0].EventID
	e.clearPrefix("1|", map[string]bool{})
	a = e.Alerts()
	if len(a) != 1 || a[0].Active || a[0].ClearedAt == nil || a[0].State != "latched" || e.Overview(false).Alerts[model.SevWarning] != 1 {
		t.Fatalf("復旧未確認: %+v", a)
	}
	e.raise(map[string]bool{}, key, ev)
	if e.Alerts()[0].EventID != id || codes(t, st)["TEST"] != 1 {
		t.Fatal("未確認再発生でイベントが増えた")
	}
	if err := e.Ack(id, false); err != nil {
		t.Fatal(err)
	}
	if a = e.Alerts(); len(a) != 1 || !a[0].Acked || !a[0].Active {
		t.Fatalf("発生中の確認: %+v", a)
	}
	e.clearPrefix("1|", map[string]bool{})
	if len(e.Alerts()) != 0 {
		t.Fatal("確認済み復旧が残った")
	}
	e.raise(map[string]bool{}, key, ev)
	if e.Alerts()[0].EventID == id {
		t.Fatal("確認・復旧後の新規発生が旧ID")
	}
	e.clearPrefix("1|", map[string]bool{})
	if err := e.Ack(0, true); err != nil {
		t.Fatal(err)
	}
	if len(e.Alerts()) != 0 {
		t.Fatal("復旧後の一括確認が残った")
	}
	if !h.has("alerts_changed") {
		t.Fatal("変更通知なし")
	}
	list, err := st.Events(store.EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range list {
		if ev.Code == "TEST" && !ev.Acked {
			t.Fatal("DBが未確認")
		}
	}
}

func TestAlertRestoreScopeAgeNewest(t *testing.T) {
	e, st := newTestEngine(t)
	pid := e.ProfileID()
	p2, err := st.CreateProfile("別現場")
	if err != nil {
		t.Fatal(err)
	}
	sw := model.SwitchConfig{ProfileID: p2, Name: "OTHER", Enabled: true}
	if err := st.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()
	var newest int64
	for i, ev := range []model.Event{
		{SwitchID: 1, Severity: model.SevWarning, Code: "RESTORE", TS: base.Add(-2 * time.Hour)},
		{SwitchID: 1, Severity: model.SevWarning, Code: "RESTORE", TS: base.Add(-time.Hour)},
		{SwitchID: 1, Severity: model.SevWarning, Code: "TOO_OLD", TS: base.Add(-37 * time.Hour)},
		{SwitchID: sw.ID, Severity: model.SevCritical, Code: "OTHER", TS: base},
		{SwitchID: 1, Severity: model.SevInfo, Code: "INFO", TS: base},
		{SwitchID: 1, Severity: model.SevWarning, Code: "ACKED", TS: base},
	} {
		ev.Category = "ANOMALY"
		ev.Title = "検証"
		if _, err := st.InsertEvent(&ev); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			newest = ev.ID
		}
		if ev.Code == "ACKED" {
			if err := st.Ack(ev.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
	e = New(st, pid, nil)
	if a := e.Alerts(); len(a) != 1 || a[0].EventID != newest || a[0].Active || a[0].State != "latched" {
		t.Fatalf("復元: %+v", a)
	}
	e.raise(map[string]bool{}, "1|WARNING|RESTORE|0|medium", alertEvent("RESTORE", 0))
	if a := e.Alerts(); len(a) != 1 || !a[0].Active || a[0].EventID != newest {
		t.Fatalf("再評価: %+v", a)
	}
}

func TestPhaseSuppressionAndLegacyMigration(t *testing.T) {
	e, st := newTestEngine(t)
	h := &recordingHub{}
	e.hub = h
	for _, phase := range []string{"setup", "rehearsal", "show"} {
		if err := e.SetPhase(phase); err != nil {
			t.Fatal(err)
		}
		e.raise(map[string]bool{}, "port", alertEvent("PORT", 1))
		e.raise(map[string]bool{}, "chassis", alertEvent("CPU_HIGH", 0))
		want := 2
		if phase == "setup" {
			want = 1
		}
		if got := e.Overview(false).Alerts[model.SevWarning]; got != want {
			t.Fatalf("%s: got %d want %d", phase, got, want)
		}
	}
	if !h.has("phase_changed") || h.has("work_changed") {
		t.Fatal("運用変更通知が不正")
	}
	if e.SetPhase("teardown") == nil || e.SetPhase("invalid") == nil {
		t.Fatal("廃止・不正フェーズを受理")
	}
	if _, err := st.DB.Exec(`INSERT OR REPLACE INTO meta(k,v) VALUES('op_phase','teardown'),('work_mode',?)`, fmt.Sprintf(`{"*":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))); err != nil {
		t.Fatal(err)
	}
	restored := New(st, e.ProfileID(), nil)
	if restored.Phase() != "setup" {
		t.Fatal("撤収は設営へ移行")
	}
	if err := restored.SetPhase("show"); err != nil {
		t.Fatal(err)
	}
	restored.raise(map[string]bool{}, "chassis", alertEvent("CPU_HIGH", 0))
	for _, alert := range restored.Alerts() {
		if alert.Suppressed {
			t.Fatal("旧作業状態で抑止された")
		}
	}
}

func registerDevice(t *testing.T, e *Engine, st *store.Store, mac, ip string) {
	t.Helper()
	if err := st.SaveAlias(e.ProfileID(), store.Alias{MatchType: "mac", MatchKey: mac, Label: "卓", PlannedIP: ip}); err != nil {
		t.Fatal(err)
	}
	e.Reload()
}

func hasAlert(e *Engine, code string) bool {
	for _, a := range e.Alerts() {
		if a.Code == code && a.Active {
			return true
		}
	}
	return false
}

func TestRegisteredMissingAndPlannedIP(t *testing.T) {
	e, st := newTestEngine(t)
	mac := "00:11:22:33:44:55"
	registerDevice(t, e, st, mac, "192.0.2.10")
	registerDevice(t, e, st, "00:11:22:33:44:66", "")
	s := &model.SwitchState{ID: 1, Name: "SW", Reachable: true, Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Oper: "up"}}}
	e.OnState(s, "slow")
	e.OnState(s, "slow")
	if hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("未観測登録機器の消失")
	}
	s.Ports[0].Devices = []model.Device{{MAC: mac, IP: "192.0.2.11", Source: "fdb"}}
	e.OnState(s, "slow")
	if !hasAlert(e, "IP_MISMATCH") {
		t.Fatal("予定IP不一致なし")
	}
	s.Ports[0].Devices = nil
	e.OnState(s, "slow")
	if hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("1回で消失")
	}
	s.Reachable = false
	e.OnState(s, "error")
	s.Reachable = true
	e.OnState(s, "slow")
	if hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("到達不能を挟んで連続扱い")
	}
	e.OnState(s, "slow")
	if !hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("2回不在で消失なし")
	}
	for _, a := range e.Alerts() {
		if a.Code == "DEVICE_MISSING" && (a.MAC != mac || a.Port != "1/0/1" || a.DeviceName != "卓") {
			t.Fatalf("詳細: %+v", a)
		}
	}
	s.Ports[0].Devices = []model.Device{{MAC: mac, IP: "192.0.2.10", Source: "fdb"}}
	e.OnState(s, "slow")
	if hasAlert(e, "DEVICE_MISSING") || hasAlert(e, "IP_MISMATCH") {
		t.Fatal("復旧していない")
	}
	if err := e.Ack(0, true); err != nil {
		t.Fatal(err)
	}
	e.OnEvent(model.Event{SwitchID: 1, IfIndex: 1, Code: "LINK_DOWN", Category: "CONNECT", Severity: model.SevWarning, Title: "リンク断"})
	if !hasAlert(e, "LINK_DOWN") {
		t.Fatal("既存リンク断が即時反映されない")
	}
	e.OnEvent(model.Event{SwitchID: 1, IfIndex: 1, Code: "LINK_UP", Category: "CONNECT", Severity: model.SevInfo, Title: "復帰"})
	if hasAlert(e, "LINK_DOWN") {
		t.Fatal("リンク復帰していない")
	}
}

type fakePinger struct {
	mu                sync.Mutex
	status            ping.Status
	calls             int
	active, maxActive int
	block             bool
	started           chan struct{}
}

func (p *fakePinger) One(ctx context.Context, ip string, _ time.Duration) ping.Result {
	p.mu.Lock()
	p.calls++
	p.active++
	if p.active > p.maxActive {
		p.maxActive = p.active
	}
	status, block := p.status, p.block
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.active--; p.mu.Unlock() }()
	if p.started != nil {
		select {
		case p.started <- struct{}{}:
		default:
		}
	}
	if block {
		<-ctx.Done()
	}
	return ping.Result{IP: ip, Status: status, RTTms: 4}
}
func (p *fakePinger) count() int { p.mu.Lock(); defer p.mu.Unlock(); return p.calls }

func TestPeriodicPingPhasesFailuresRecovery(t *testing.T) {
	e, st := newTestEngine(t)
	mac := "00:11:22:33:44:55"
	registerDevice(t, e, st, mac, "192.0.2.10")
	p := &fakePinger{status: ping.StatusTimeout}
	e.SetPinger(p)
	e.periodicPing(context.Background())
	if p.count() != 0 {
		t.Fatal("本番で定期送信")
	}
	if err := e.SetPhase("rehearsal"); err != nil {
		t.Fatal(err)
	}
	e.periodicPing(context.Background())
	if hasAlert(e, "PING_FAIL") {
		t.Fatal("失敗1回で発報")
	}
	e.periodicPing(context.Background())
	if !hasAlert(e, "PING_FAIL") {
		t.Fatal("失敗2回で発報なし")
	}
	rows, err := e.Inventory(InventoryOptions{})
	if err != nil || len(rows) != 1 || rows[0].PingOK == nil || *rows[0].PingOK || rows[0].PingTS == nil {
		t.Fatalf("ping行: %+v %v", rows, err)
	}
	p.mu.Lock()
	p.status = ping.StatusOK
	p.mu.Unlock()
	e.periodicPing(context.Background())
	if hasAlert(e, "PING_FAIL") || len(e.Alerts()) != 1 || e.Alerts()[0].State != "latched" {
		t.Fatal("Ping復旧時のラッチ")
	}
	if err := e.SetPhase("show"); err != nil {
		t.Fatal(err)
	}
	before := p.count()
	e.periodicPing(context.Background())
	if p.count() != before {
		t.Fatal("本番で送信")
	}
	if err := e.SetPhase("setup"); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	p.status = ping.StatusTimeout
	p.mu.Unlock()
	e.periodicPing(context.Background())
	e.periodicPing(context.Background())
	if !hasAlert(e, "PING_FAIL") || e.Overview(false).Alerts[model.SevWarning] != 0 {
		t.Fatal("設営Pingまたは抑止")
	}
	if e.pingResults[mac].Source != "periodic" {
		t.Fatal("sourceなし")
	}
	e.RecordManualPing(e.ProfileID(), map[string]PingMark{"192.0.2.10": {Status: "ok", RTTms: 1, At: time.Now()}})
	if p := e.pingResults[mac]; !p.OK || p.Source != "manual" {
		t.Fatal("手動結果なし")
	}
}

func TestPeriodicPingCancellationAndParallelBound(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprint(shutdown), func(t *testing.T) {
			e, st := newTestEngine(t)
			for i := 0; i < 40; i++ {
				if err := st.SaveAlias(e.ProfileID(), store.Alias{MatchType: "mac", MatchKey: fmt.Sprintf("02:00:00:00:00:%02X", i), Label: "機器", PlannedIP: fmt.Sprintf("192.0.2.%d", i+1)}); err != nil {
					t.Fatal(err)
				}
			}
			p := &fakePinger{status: ping.StatusTimeout, block: true, started: make(chan struct{}, 40)}
			e.SetPinger(p)
			if err := e.SetPhase("setup"); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			go func() { defer close(done); e.runPeriodicPing(ctx, func() time.Duration { return time.Millisecond }) }()
			for i := 0; i < ping.MaxParallel; i++ {
				select {
				case <-p.started:
				case <-time.After(3 * time.Second):
					t.Fatal("16並列が開始しない")
				}
			}
			if shutdown {
				cancel()
			} else {
				if err := e.SetPhase("show"); err != nil {
					t.Fatal(err)
				}
				cancel()
			}
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("停止しない")
			}
			p.mu.Lock()
			active, max := p.active, p.maxActive
			p.mu.Unlock()
			if active != 0 || max > 16 {
				t.Fatalf("active=%d max=%d", active, max)
			}
			if len(e.pingResults) != 0 || hasAlert(e, "PING_FAIL") {
				t.Fatal("中断した結果が反映された")
			}
		})
	}
}

func TestMissingFDBFailureAndRawFDB(t *testing.T) {
	e, st := newTestEngine(t)
	mac := "00:11:22:33:44:55"
	registerDevice(t, e, st, mac, "")
	valid := true
	s := &model.SwitchState{ID: 1, Name: "SW", Reachable: true, FDBValid: &valid, FDBMACs: []string{mac}, Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Oper: "up", Devices: []model.Device{{MAC: mac, Source: "lldp"}}}}}
	e.OnState(s, "slow")
	s.FDBMACs = nil
	e.OnState(s, "slow")
	if hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("不在1回")
	}
	valid = false
	e.OnState(s, "slow")
	valid = true
	e.OnState(s, "slow")
	if hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("FDB取得失敗を挟んだのに連続扱い")
	}
	e.OnState(s, "slow")
	if !hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("LLDPに残ってもFDB不在2回なら消失")
	}
	s.FDBMACs = []string{mac}
	e.OnState(s, "slow")
	if hasAlert(e, "DEVICE_MISSING") {
		t.Fatal("FDB復帰が反映されない")
	}
}

func TestRestoredEdgesAndOldProfileUpdates(t *testing.T) {
	e, st := newTestEngine(t)
	e.OnEvent(model.Event{SwitchID: 1, IfIndex: 1, Code: "LINK_DOWN", Category: "CONNECT", Severity: model.SevWarning, Title: "リンク断"})
	e.OnEvent(model.Event{SwitchID: 1, Code: "SWITCH_UNREACHABLE", Category: "ANOMALY", Severity: model.SevCritical, Title: "到達不能"})
	e = New(st, e.ProfileID(), nil)
	e.OnState(&model.SwitchState{ID: 1, Reachable: false, LastPoll: time.Now()}, "error")
	if !hasAlert(e, "SWITCH_UNREACHABLE") {
		t.Fatal("継続障害の再評価なし")
	}
	e.OnState(&model.SwitchState{ID: 1, Reachable: true, Ports: []*model.Port{{IfIndex: 1, Oper: "down"}}}, "fast")
	if !hasAlert(e, "LINK_DOWN") || hasAlert(e, "SWITCH_UNREACHABLE") {
		t.Fatal("継続リンク断・復帰の再評価なし")
	}
	pid, err := st.CreateProfile("次の現場")
	if err != nil {
		t.Fatal(err)
	}
	e.SetProfile(pid)
	e.OnState(&model.SwitchState{ID: 1, Reachable: true}, "slow")
	e.OnEvent(model.Event{SwitchID: 1, IfIndex: 2, Code: "LINK_DOWN", Category: "CONNECT", Severity: model.SevWarning, Title: "旧現場"})
	if len(e.States()) != 0 || len(e.Alerts()) != 0 {
		t.Fatal("旧プロファイルのポーラー出力が混入")
	}
}

func TestDevicePingRestoreAfterPortMove(t *testing.T) {
	e, st := newTestEngine(t)
	mac := "00:11:22:33:44:55"
	registerDevice(t, e, st, mac, "192.0.2.10")
	if err := e.SetPhase("rehearsal"); err != nil {
		t.Fatal(err)
	}
	s := &model.SwitchState{ID: 1, Name: "SW", Reachable: true, Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Oper: "up", Devices: []model.Device{{MAC: mac, IP: "192.0.2.10"}}}}}
	e.OnState(s, "slow")
	e.SetPinger(&fakePinger{status: ping.StatusTimeout})
	e.periodicPing(context.Background())
	e.periodicPing(context.Background())
	id := e.Alerts()[0].EventID
	s.Ports[0].IfIndex, s.Ports[0].Name = 2, "1/0/2"
	e.OnState(s, "slow")
	e.periodicPing(context.Background())
	e = New(st, e.ProfileID(), nil)
	e.OnState(s, "slow")
	e.SetPinger(&fakePinger{status: ping.StatusTimeout})
	e.periodicPing(context.Background())
	e.periodicPing(context.Background())
	a := e.Alerts()
	if len(a) != 1 || !a[0].Active || a[0].EventID != id || a[0].IfIndex != 2 || a[0].Port != "1/0/2" {
		t.Fatalf("移動後の再起動: %+v", a)
	}
}

func TestRestoredWarningEscalatesToCritical(t *testing.T) {
	e, st := newTestEngine(t)
	s := &model.SwitchState{ID: 1, Name: "SW", Reachable: true, Health: model.Health{TempState: "warning", TempMaxC: 70}}
	e.OnState(s, "medium")
	e = New(st, e.ProfileID(), nil)
	s.Health.TempState = "critical"
	e.OnState(s, "medium")
	critical, warning := 0, 0
	for _, a := range e.Alerts() {
		if a.Code != "TEMP_HIGH" {
			continue
		}
		if a.Severity == model.SevCritical && a.Active {
			critical++
		}
		if a.Severity == model.SevWarning && !a.Active {
			warning++
		}
	}
	if critical != 1 || warning != 1 || e.Overview(false).Alerts[model.SevCritical] != 1 {
		t.Fatalf("重要度の再評価: %+v", e.Alerts())
	}
}

func TestDeviceRestoreUsesLatestGroup(t *testing.T) {
	e, st := newTestEngine(t)
	mac := "00:11:22:33:44:55"
	registerDevice(t, e, st, mac, "192.0.2.10")
	if err := e.SetPhase("rehearsal"); err != nil {
		t.Fatal(err)
	}
	first := &model.SwitchState{ID: 1, Name: "SW", Reachable: true, Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Oper: "up", Devices: []model.Device{{MAC: mac, IP: "192.0.2.10"}}}}}
	e.OnState(first, "slow")
	e.SetPinger(&fakePinger{status: ping.StatusTimeout})
	e.periodicPing(context.Background())
	e.periodicPing(context.Background())
	sw := model.SwitchConfig{ProfileID: e.ProfileID(), Name: "SW2", GroupTag: "作業班", Enabled: true}
	if err := st.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	if err := e.SetPhase("setup"); err != nil {
		t.Fatal(err)
	}
	// DBのミリ秒精度に依存せず、移動前後の順序を固定する。
	if _, err := st.DB.Exec(`UPDATE device_bindings SET last_seen_ts=last_seen_ts-1000 WHERE switch_id=1`); err != nil {
		t.Fatal(err)
	}
	second := &model.SwitchState{ID: sw.ID, Name: sw.Name, GroupTag: sw.GroupTag, Reachable: true, Ports: []*model.Port{{IfIndex: 2, Name: "1/0/2", Oper: "up", Devices: first.Ports[0].Devices}}}
	e.OnState(second, "slow")
	e = New(st, e.ProfileID(), nil)
	for _, a := range e.Alerts() {
		if a.Code == "PING_FAIL" {
			t.Fatalf("移動前のグループで再ラッチ: %+v", a)
		}
	}
}
