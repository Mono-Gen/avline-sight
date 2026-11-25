// Package poller は登録スイッチを3段階周期で並列ポーリングし、正規化済み状態とイベントを生成する。
package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/model"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

// Intervals は3段階周期（仕様2.3-1）。
type Intervals struct {
	Fast, Medium, Slow time.Duration
	Timeout            time.Duration
	Retries            int
}

// DefaultIntervals は仕様の既定値。
var DefaultIntervals = Intervals{Fast: 3 * time.Second, Medium: 12 * time.Second, Slow: 45 * time.Second, Timeout: 1500 * time.Millisecond, Retries: 1}

// Sink はポーラーの出力先（エンジン）。並行に呼ばれるため実装側で排他すること。
type Sink interface {
	OnState(st *model.SwitchState, tier string)
	OnEvent(ev model.Event)
}

// AliasFunc は手動ラベル解決（ifIndex, MAC）→ラベル。
type AliasFunc func(switchID int64, ifIndex int, mac string) string

// Manager は全ワーカーを管理する。
type Manager struct {
	mu      sync.Mutex
	iv      Intervals
	sink    Sink
	alias   AliasFunc
	workers map[int64]*worker
	phase   func() string
}

// NewManager を作る。
func NewManager(iv Intervals, sink Sink, alias AliasFunc) *Manager {
	return &Manager{iv: iv, sink: sink, alias: alias, workers: map[int64]*worker{}}
}

// WithPhase はワーカー起動前に運用フェーズの読取元を設定する。
func (m *Manager) WithPhase(phase func() string) *Manager { m.phase = phase; return m }

// Sync は登録スイッチ一覧に合わせてワーカーを起動・停止する。
func (m *Manager) Sync(ctx context.Context, sws []model.SwitchConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int64]model.SwitchConfig{}
	for _, s := range sws {
		if s.Enabled && !s.NeedsCredentials() {
			want[s.ID] = s
		}
	}
	for id, w := range m.workers {
		cfg, ok := want[id]
		if !ok || cfg.IP != w.cfg.IP || cfg.SNMPPort != w.cfg.SNMPPort || cfg.Community != w.cfg.Community {
			w.cancel()
			delete(m.workers, id)
		} else {
			w.mu.Lock()
			w.cfg = cfg
			w.mu.Unlock()
		}
	}
	for id, cfg := range want {
		if _, ok := m.workers[id]; ok {
			continue
		}
		wctx, cancel := context.WithCancel(ctx)
		w := newWorker(cfg, m.iv, m.sink, m.alias, cancel)
		w.phase = m.phase
		m.workers[id] = w
		go w.run(wctx)
	}
}

// ResetCounters はソフト・カウンタゼロリセット（実機カウンタは消さない）。ifIndex=0で全ポート。
func (m *Manager) ResetCounters(switchID int64, ifIndex int) bool {
	m.mu.Lock()
	w := m.workers[switchID]
	m.mu.Unlock()
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range w.st.Ports {
		if ifIndex == 0 || p.IfIndex == ifIndex {
			p.ErrSinceReset, p.DiscardSinceReset = 0, 0
		}
	}
	return true
}

// Probe は全機能グループを再判定させる（次回Slowで再取得）。
func (m *Manager) Probe(switchID int64) {
	m.mu.Lock()
	w := m.workers[switchID]
	m.mu.Unlock()
	if w != nil {
		w.mu.Lock()
		w.st.Caps = map[string]string{}
		w.mu.Unlock()
		select {
		case w.probeNow <- struct{}{}:
		default:
		}
	}
}

// StopAll は全ワーカーを停止する。
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, w := range m.workers {
		w.cancel()
		delete(m.workers, id)
	}
}

type worker struct {
	mu           sync.Mutex
	cfg          model.SwitchConfig
	iv           Intervals
	sink         Sink
	alias        AliasFunc
	cancel       context.CancelFunc
	probeNow     chan struct{}
	requests     requests
	phase        func() string
	events       []model.Event
	ifLastChange map[int]uint64
	flapChange   map[int]uint64
	configRead   map[int]map[string]string
	configFailed map[string]bool

	c           *sx.Client
	st          *model.SwitchState
	cnt         counters
	lastUptime  int64
	basePort    map[int]int
	vlanNames   map[int]string
	lastLogIdx  int
	logPrimed   bool // 内部ログを一度でも取得したか（初回分は監視開始前のログ）
	devSeen     map[int]string // ifIndex → MAC集合の署名（接続変化検知）
	failures    int
	lastUpSpeed map[int]int // ポート毎の直近リンクアップ速度
	slowCount   int
	uptimeAt    time.Time // lastUptime を取得した時刻
}

// reprobeEverySlow はSlow周期何回ごとに非対応グループを再確認するか（既定45秒×10≒7.5分）。
const reprobeEverySlow = 10

func newWorker(cfg model.SwitchConfig, iv Intervals, sink Sink, alias AliasFunc, cancel context.CancelFunc) *worker {
	return &worker{cfg: cfg, iv: iv, sink: sink, alias: alias, cancel: cancel, probeNow: make(chan struct{}, 1), requests: requests{wake: make(chan struct{}, 1)},
		st:  &model.SwitchState{ID: cfg.ID, Name: cfg.Name, IP: cfg.IP, GroupTag: cfg.GroupTag, Caps: map[string]string{}},
		cnt: counters{vals: map[string]uint64{}}, basePort: map[int]int{}, vlanNames: map[int]string{}, devSeen: map[int]string{}, lastUpSpeed: map[int]int{}}
}

func (w *worker) emit(ev model.Event) {
	ev.Hint = model.EventHintJa(ev.Code)
	ev.SwitchID = w.cfg.ID
	ev.SwitchName = w.cfg.Name
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	w.events = append(w.events, ev)
}

func (w *worker) supported(group string) bool { return w.st.Caps[group] != "UNSUPPORTED" }

// markCap は機能グループの対応状況を記録する。初回に行が0件なら非対応として以後スキップ（仕様7.2）。
func (w *worker) markCap(group string, err error, n int) {
	if _, known := w.st.Caps[group]; known && w.st.Caps[group] != "" {
		return
	}
	switch {
	case err != nil:
		// 通信エラーは判定保留（タイムアウト等は非対応と区別する）
	case n == 0:
		w.st.Caps[group] = "UNSUPPORTED"
	default:
		w.st.Caps[group] = "SUPPORTED"
	}
}

func (w *worker) aliasFor(ifIndex int, mac string) string {
	if w.alias == nil {
		return ""
	}
	return w.alias(w.cfg.ID, ifIndex, mac)
}

// snapshot はシンク向けに状態のディープコピーを返す（JSON往復で安全に複製）。
func (w *worker) snapshot() *model.SwitchState {
	b, err := json.Marshal(w.st)
	if err != nil {
		slog.Error("状態の複製に失敗", "err", err)
		return &model.SwitchState{ID: w.cfg.ID, Name: w.cfg.Name}
	}
	var cp model.SwitchState
	if err := json.Unmarshal(b, &cp); err != nil {
		slog.Error("状態の複製に失敗", "err", err)
	}
	// FDBの内部観測は配信・再生に含めない。次の取得ではマップごと差し替える。
	cp.FDBLastOK, cp.FDBPorts = w.st.FDBLastOK, w.st.FDBPorts
	for i, port := range cp.Ports {
		port.RawCounters = maps.Clone(w.st.Ports[i].RawCounters)
	}
	cp.MbufFailCounters, cp.DHCPInvalidServerCounters = w.st.MbufFailCounters, w.st.DHCPInvalidServerCounters
	return &cp
}

func (w *worker) run(ctx context.Context) {
	w.mu.Lock()
	cfg := w.cfg
	w.mu.Unlock()
	port := uint16(cfg.SNMPPort)
	if port == 0 {
		port = 161
	}
	c, err := dialSNMP(ctx, cfg.IP, port, cfg.Community, w.iv.Timeout, w.iv.Retries)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		w.mu.Lock()
		w.st.Reachable, w.st.LastError = false, err.Error()
		snap := w.snapshot()
		w.mu.Unlock()
		w.sink.OnState(snap, "error")
		return
	}
	w.c = c
	defer func() { w.c.Close() }()
	// 取得に失敗したらUDPソケットを作り直す。監視PCのLANアダプタが落ちて戻ると（USBアダプタの抜き差し・固まり）、
	// 起動時に作ったソケットは古い経路のまま送れず、Pingが戻っても再起動までタイムアウトし続けた（2026-11-25 実機で確認）。
	// UDPソケットの作成は軽く、再試行は5〜30秒間隔なので負荷は増えない。作れなければ今のソケットを使い続ける。
	redial := func() {
		nc, err := dialSNMP(ctx, cfg.IP, port, cfg.Community, w.iv.Timeout, w.iv.Retries)
		if err != nil {
			return
		}
		w.mu.Lock()
		old := w.c
		w.c = nc
		w.mu.Unlock()
		old.Close()
	}

	tiers := []struct {
		name string
		iv   time.Duration
		fn   func() error
		last time.Time
	}{
		{name: "fast", iv: w.iv.Fast, fn: w.collectFast},
		{name: "slow", iv: w.iv.Slow, fn: w.collectSlow},
		{name: "medium", iv: w.iv.Medium, fn: w.collectMedium},
	}
	backoff := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}
	var retryAt time.Time
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.probeNow:
			retryAt = time.Time{}
			for i := range tiers {
				tiers[i].last = time.Time{}
			}
		case <-w.requests.wake:
		case <-tick.C:
		}
		now := time.Now()
		if now.Before(retryAt) {
			continue
		}
		phase := "show"
		if w.phase != nil {
			phase = w.phase()
		}
		retry := !retryAt.IsZero()
		trapFast, trapMedium, trapSlow := false, false, false
		if !retry {
			trapFast, trapMedium, trapSlow = w.requests.takeTrap(now)
		}
		for i := range tiers {
			if ctx.Err() != nil {
				return
			}
			t := &tiers[i]
			if retry && t.name != "slow" {
				continue
			}
			if t.name == "medium" && w.requests.takeFDB(time.Now(), false) {
				w.poll(ctx, "fdb", w.collectFDBOnly)
				if ctx.Err() != nil {
					return
				}
			}
			due := !now.Before(t.last.Add(intervalForPhase(t.iv, t.name, phase)))
			forced := t.name == "fast" && trapFast ||
				t.name == "medium" && trapMedium ||
				t.name == "slow" && (trapSlow || retry)
			if !due && !forced {
				continue
			}
			if t.name == "slow" {
				w.requests.takeFDB(time.Now(), true)
			}
			t.last = time.Now()
			if !w.poll(ctx, t.name, t.fn) {
				if ctx.Err() != nil {
					return
				}
				retryAt = time.Now().Add(backoff[min(w.failures-1, len(backoff)-1)])
				redial()
				break
			}
			retryAt = time.Time{}
		}
	}
}

func (w *worker) poll(ctx context.Context, tier string, collect func() error) bool {
	w.mu.Lock()
	w.configRead = nil
	w.configFailed = nil
	err := collect()
	if ctx.Err() != nil {
		w.events = nil
		w.mu.Unlock()
		return false
	}
	w.st.LastPoll = time.Now().UTC()
	// FDB単独の失敗は不在判定を中断するが、スイッチ全体の死活は通常取得で判定する。
	if tier != "fdb" {
		if err != nil {
			wasUp := w.st.Reachable
			w.failures++
			w.st.Reachable = false
			w.st.LastError = friendlyErr(err)
			for _, p := range w.st.Ports {
				clear(p.ConfigSettings)
			}
			if wasUp || w.failures == 1 {
				w.emit(model.Event{Category: "ANOMALY", Severity: model.SevCritical, Code: "SWITCH_UNREACHABLE",
					Title:  "スイッチからSNMP応答がありません",
					Detail: map[string]any{"error": w.st.LastError}})
			}
			tier = "error"
		} else {
			if w.failures > 0 {
				w.emit(model.Event{Category: "ANOMALY", Severity: model.SevInfo, Code: "SWITCH_RECOVERED", Title: "スイッチのSNMP応答が復帰しました"})
			}
			w.failures = 0
			w.st.Reachable, w.st.LastError = true, ""
		}
	}
	if err == nil {
		w.st.LastOK = w.st.LastPoll
		w.finishConfigCollection(tier)
	}
	snap, events := w.snapshot(), w.events
	w.events = nil
	w.mu.Unlock()
	// コールバックから同じワーカーへ追加取得を要求できるよう、ロック外で通知する。
	for _, ev := range events {
		w.sink.OnEvent(ev)
	}
	w.sink.OnState(snap, tier)
	return err == nil
}

func friendlyErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "timeout") || strings.Contains(s, "i/o timeout") {
		return "タイムアウト（応答なし）: " + s
	}
	return s
}

// ---- 状態変化イベント ----

func (w *worker) observeLinkFlap(p *model.Port, oper string, signals linkSignals, changes map[int]uint64) {
	if oper != "up" {
		return
	}
	prev, havePrev := w.ifLastChange[p.IfIndex]
	cur, haveCur := changes[p.IfIndex]
	advanced := p.Oper == "up" && havePrev && haveCur && cur > prev
	var count int
	if p.Oper == "" || p.Oper == "up" {
		covered, haveCovered := w.flapChange[p.IfIndex]
		for _, tick := range signals.downs {
			// 同じ変化をifLastChangeで先に確認した場合、遅れて受け取ったTrapを重ねない。
			if tick != nil && haveCovered && havePrev && haveCur && cur >= prev && covered == prev && *tick <= covered {
				continue
			}
			count++
		}
	}
	var source string
	switch {
	case count > 0:
		source = "trap"
	case advanced:
		source = "ifLastChange"
		count = 1
	default:
		return
	}
	if advanced {
		if w.flapChange == nil {
			w.flapChange = map[int]uint64{}
		}
		w.flapChange[p.IfIndex] = cur
	}
	w.emit(model.Event{Category: "ANOMALY", Severity: model.SevWarning, Code: "LINK_FLAP", IfIndex: p.IfIndex,
		Title:  p.Name + " 短時間のリンク断を検知",
		Detail: map[string]any{"port": p.Name, "source": source, "count": count}})
}

func (w *worker) onLinkChange(p *model.Port, oper string, spd int) {
	p.LastChange = time.Now().UTC()
	switch {
	case p.Oper == "up" && oper != "up":
		// リンク断は登録の有無に関係なく重大（2026-11-25 ユーザー決定）。登録漏れの機器が抜けても軽く見えないように色を統一する。
		// 設営フェーズではポートのアラート自体を抑止するので、押し引きで赤くはならない。
		w.emit(model.Event{Category: "CONNECT", Severity: model.SevCritical, Code: "LINK_DOWN", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s リンクダウン", p.Name), Detail: map[string]any{"port": p.Name}})
	case p.Oper != "up" && oper == "up":
		// LinkDown → 低速で再接続（例 1G→100M）は芯線断線の典型（仕様3.6）
		if prev := w.lastUpSpeed[p.IfIndex]; prev > 0 && spd > 0 && spd < prev {
			w.emit(model.Event{Category: "ANOMALY", Severity: model.SevCritical, Code: "RELINK_SLOWER", IfIndex: p.IfIndex,
				Title:  fmt.Sprintf("%s 再接続後に速度低下 %dM → %dM", p.Name, prev, spd),
				Detail: map[string]any{"from": prev, "to": spd}})
		}
		w.emit(model.Event{Category: "CONNECT", Severity: model.SevInfo, Code: "LINK_UP", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s リンクアップ（%dMbps）", p.Name, spd), Detail: map[string]any{"port": p.Name, "speed": spd}})
	case p.SpeedMbps != spd:
		sev := model.SevInfo
		if spd < p.SpeedMbps {
			sev = model.SevCritical
		}
		w.emit(model.Event{Category: "CONNECT", Severity: sev, Code: "SPEED_CHANGE", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s 速度変化 %dM → %dM", p.Name, p.SpeedMbps, spd), Detail: map[string]any{"from": p.SpeedMbps, "to": spd}})
	}
}

func (w *worker) onPoEChange(p *model.Port, prev, cur *model.PoE, cols map[string]gosnmp.SnmpPDU) {
	if prev.Detect != cur.Detect {
		sev := model.SevInfo
		if cur.Detect == "fault" || cur.Detect == "otherFault" {
			sev = model.SevCritical
		}
		w.emit(model.Event{Category: "CONNECT", Severity: sev, Code: "POE_STATE", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s PoE状態 %s → %s", p.Name, prev.Detect, cur.Detect), Detail: map[string]any{"fault": cur.Fault}})
	}
	if cur.OverloadCnt > prev.OverloadCnt || cur.DeniedCnt > prev.DeniedCnt || cur.ShortCnt > prev.ShortCnt {
		w.emit(model.Event{Category: "ANOMALY", Severity: model.SevCritical, Code: "POE_FAULT", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s PoE障害（Overload/Denied/Short）", p.Name),
			Detail: map[string]any{"overload": cur.OverloadCnt, "denied": cur.DeniedCnt, "short": cur.ShortCnt,
				"raw_counters": captureCounters(nil, cols, poeCounters)}})
	}
}

func (w *worker) onDevicesChange(p *model.Port, list []model.Device) {
	var macs []string
	for _, d := range list {
		macs = append(macs, d.MAC)
	}
	sig := strings.Join(macs, ",")
	prev, seen := w.devSeen[p.IfIndex]
	w.devSeen[p.IfIndex] = sig
	if !seen || prev == sig {
		return
	}
	old := map[string]bool{}
	for _, m := range strings.Split(prev, ",") {
		if m != "" {
			old[m] = true
		}
	}
	for _, d := range list {
		if !old[d.MAC] {
			name := d.LLDPName
			if name == "" {
				name = d.Vendor
			}
			w.emit(model.Event{Category: "CONNECT", Severity: model.SevInfo, Code: "DEVICE_NEW", IfIndex: p.IfIndex,
				Title: fmt.Sprintf("%s に機器を検出 %s %s", p.Name, d.MAC, name), Detail: map[string]any{"mac": d.MAC, "ip": d.IP, "vendor": d.Vendor}})
		}
		delete(old, d.MAC)
	}
	for m := range old {
		w.emit(model.Event{Category: "CONNECT", Severity: model.SevInfo, Code: "DEVICE_GONE", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s から機器が消失 %s", p.Name, m), Detail: map[string]any{"mac": m}})
	}
}

// dialSNMP はテストで作り直しの回数を数えられるよう差し替え可能にしている。
var dialSNMP = sx.DialContext
