// Package poller は登録スイッチを3段階周期で並列ポーリングし、正規化済み状態とイベントを生成する。
package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

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
}

// NewManager を作る。
func NewManager(iv Intervals, sink Sink, alias AliasFunc) *Manager {
	return &Manager{iv: iv, sink: sink, alias: alias, workers: map[int64]*worker{}}
}

// Sync は登録スイッチ一覧に合わせてワーカーを起動・停止する。
func (m *Manager) Sync(ctx context.Context, sws []model.SwitchConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	want := map[int64]model.SwitchConfig{}
	for _, s := range sws {
		if s.Enabled {
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
		m.workers[id] = w
		go w.run(wctx)
	}
}

// ResetCounters はソフト・カウンタゼロリセット（実機カウンタは消さない）。ifIndex=0で全ポート。
func (m *Manager) ResetCounters(switchID int64, ifIndex int) {
	m.mu.Lock()
	w := m.workers[switchID]
	m.mu.Unlock()
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range w.st.Ports {
		if ifIndex == 0 || p.IfIndex == ifIndex {
			p.ErrSinceReset, p.DiscardSinceReset = 0, 0
		}
	}
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
	mu       sync.Mutex
	cfg      model.SwitchConfig
	iv       Intervals
	sink     Sink
	alias    AliasFunc
	cancel   context.CancelFunc
	probeNow chan struct{}

	c           *sx.Client
	st          *model.SwitchState
	cnt         counters
	lastUptime  int64
	basePort    map[int]int
	vlanNames   map[int]string
	lastLogIdx  int
	devSeen     map[int]string // ifIndex → MAC集合の署名（接続変化検知）
	failures    int
	lastUpSpeed map[int]int // ポート毎の直近リンクアップ速度
	slowCount   int
}

// reprobeEverySlow はSlow周期何回ごとに非対応グループを再確認するか（既定45秒×10≒7.5分）。
const reprobeEverySlow = 10

func newWorker(cfg model.SwitchConfig, iv Intervals, sink Sink, alias AliasFunc, cancel context.CancelFunc) *worker {
	return &worker{cfg: cfg, iv: iv, sink: sink, alias: alias, cancel: cancel, probeNow: make(chan struct{}, 1),
		st:  &model.SwitchState{ID: cfg.ID, Name: cfg.Name, IP: cfg.IP, GroupTag: cfg.GroupTag, Caps: map[string]string{}},
		cnt: counters{vals: map[string]uint64{}}, basePort: map[int]int{}, vlanNames: map[int]string{}, devSeen: map[int]string{}, lastUpSpeed: map[int]int{}}
}

func (w *worker) emit(ev model.Event) {
	ev.SwitchID = w.cfg.ID
	ev.SwitchName = w.cfg.Name
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	w.sink.OnEvent(ev)
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
	return &cp
}

func (w *worker) run(ctx context.Context) {
	host, port := w.cfg.IP, uint16(w.cfg.SNMPPort)
	if port == 0 {
		port = 161
	}
	c, err := sx.Dial(host, port, w.cfg.Community, w.iv.Timeout, w.iv.Retries)
	if err != nil {
		w.mu.Lock()
		w.st.Reachable, w.st.LastError = false, err.Error()
		w.mu.Unlock()
		w.sink.OnState(w.snapshot(), "error")
		return
	}
	defer c.Close()
	w.c = c

	tiers := []struct {
		name string
		iv   time.Duration
		fn   func() error
		next time.Time
	}{
		{"slow", w.iv.Slow, w.collectSlow, time.Time{}},
		{"fast", w.iv.Fast, w.collectFast, time.Time{}},
		{"medium", w.iv.Medium, w.collectMedium, time.Time{}},
	}
	backoff := []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.probeNow:
			for i := range tiers {
				tiers[i].next = time.Time{}
			}
		case <-tick.C:
		}
		now := time.Now()
		for i := range tiers {
			t := &tiers[i]
			if now.Before(t.next) {
				continue
			}
			w.mu.Lock()
			err := t.fn()
			w.st.LastPoll = time.Now().UTC()
			if err == nil {
				w.st.LastOK = w.st.LastPoll
			}
			wasUp := w.st.Reachable
			if err != nil {
				w.failures++
				w.st.Reachable = false
				w.st.LastError = friendlyErr(err)
				if wasUp || w.failures == 1 {
					w.emit(model.Event{Category: "ANOMALY", Severity: model.SevCritical, Code: "SWITCH_UNREACHABLE",
						Title: "スイッチからSNMP応答がありません", Hint: "電源断・LAN切断・IP/コミュニティ設定誤り・SNMP無効化を確認してください",
						Detail: map[string]any{"error": w.st.LastError}})
				}
				// 指数バックオフで死活確認へ移行（他スイッチはブロックしない）
				b := backoff[min(w.failures-1, len(backoff)-1)]
				for j := range tiers {
					tiers[j].next = now.Add(b)
				}
				tiers[0].next = now.Add(b) // 復帰確認はslow（system取得込み）から
				for j := 1; j < len(tiers); j++ {
					tiers[j].next = now.Add(b + time.Second)
				}
				snap := w.snapshot()
				w.mu.Unlock()
				w.sink.OnState(snap, "error")
				break
			}
			if !wasUp {
				if w.failures > 0 {
					w.emit(model.Event{Category: "ANOMALY", Severity: model.SevInfo, Code: "SWITCH_RECOVERED", Title: "スイッチのSNMP応答が復帰しました"})
				}
				w.failures = 0
			}
			w.st.Reachable, w.st.LastError = true, ""
			t.next = now.Add(t.iv)
			snap := w.snapshot()
			w.mu.Unlock()
			w.sink.OnState(snap, t.name)
		}
	}
}

func friendlyErr(err error) string {
	s := err.Error()
	if strings.Contains(s, "timeout") || strings.Contains(s, "i/o timeout") {
		return "タイムアウト（応答なし）: " + s
	}
	return s
}

// ---- 状態変化イベント ----

func (w *worker) onLinkChange(p *model.Port, oper string, spd int) {
	p.LastChange = time.Now().UTC()
	switch {
	case p.Oper == "up" && oper != "up":
		w.emit(model.Event{Category: "CONNECT", Severity: model.SevWarning, Code: "LINK_DOWN", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s リンクダウン", p.Name), Detail: map[string]any{"port": p.Name}})
	case p.Oper != "up" && oper == "up":
		// LinkDown → 低速で再接続（例 1G→100M）は芯線断線の典型（仕様3.6）
		if prev := w.lastUpSpeed[p.IfIndex]; prev > 0 && spd > 0 && spd < prev {
			w.emit(model.Event{Category: "ANOMALY", Severity: model.SevCritical, Code: "RELINK_SLOWER", IfIndex: p.IfIndex,
				Title: fmt.Sprintf("%s 再接続後に速度低下 %dM → %dM", p.Name, prev, spd), Hint: "LANケーブル芯線断線（4芯落ち）・コネクタ圧着不良・オートネゴ不整合の疑い",
				Detail: map[string]any{"from": prev, "to": spd}})
		}
		w.emit(model.Event{Category: "CONNECT", Severity: model.SevInfo, Code: "LINK_UP", IfIndex: p.IfIndex,
			Title: fmt.Sprintf("%s リンクアップ（%dMbps）", p.Name, spd), Detail: map[string]any{"port": p.Name, "speed": spd}})
	case p.SpeedMbps != spd:
		sev, hint := model.SevInfo, ""
		if spd < p.SpeedMbps {
			sev, hint = model.SevCritical, "LANケーブル芯線断線（4芯落ち）・コネクタ圧着不良・オートネゴ不整合の疑い"
		}
		w.emit(model.Event{Category: "CONNECT", Severity: sev, Code: "SPEED_CHANGE", IfIndex: p.IfIndex, Hint: hint,
			Title: fmt.Sprintf("%s 速度変化 %dM → %dM", p.Name, p.SpeedMbps, spd), Detail: map[string]any{"from": p.SpeedMbps, "to": spd}})
	}
}

func (w *worker) onPoEChange(p *model.Port, prev, cur *model.PoE) {
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
			Title: fmt.Sprintf("%s PoE障害（Overload/Denied/Short）", p.Name), Hint: "PoE電力不足または受電機器側の異常・短絡の疑い",
			Detail: map[string]any{"overload": cur.OverloadCnt, "denied": cur.DeniedCnt, "short": cur.ShortCnt}})
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
