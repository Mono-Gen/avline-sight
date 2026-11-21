// Package engine はポーラー出力を集約し、永続化・アラート判定・監査・ビュー生成・ライブ配信を行う。
package engine

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

// Thresholds はアラートしきい値（プロファイル単位で変更可、仕様3.6）。
type Thresholds struct {
	CPUPct     float64 `json:"cpu_pct"`
	MemFreePct float64 `json:"mem_free_pct"`
	TempC      float64 `json:"temp_c"`
	PoEPct     float64 `json:"poe_pct"`
	UtilPct    float64 `json:"util_pct"`
	SFPRxDBm   float64 `json:"sfp_rx_dbm"`
	SFPDropDB  float64 `json:"sfp_drop_db"`
	ErrDelta   uint64  `json:"err_delta"`
}

// View は担当パート別クイックビュー。
type View struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	VLANs      []int    `json:"vlans"`
	Tags       []string `json:"tags"`
	Categories []string `json:"categories"`
}

// Settings はプロファイル設定（settings_json）。
type Settings struct {
	Thresholds Thresholds `json:"thresholds"`
	AlertSound bool       `json:"alert_sound"`
	Views      []View     `json:"views"`
}

// DefaultSettings は仕様の既定値（SFP -10dBm は公式の過剰減衰基準、物理エラーは+1で要確認）。
func DefaultSettings() Settings {
	return Settings{
		Thresholds: Thresholds{CPUPct: 80, MemFreePct: 15, TempC: 65, PoEPct: 80, UtilPct: 80, SFPRxDBm: -10, SFPDropDB: 3, ErrDelta: 1},
		AlertSound: true,
		Views: []View{
			{ID: "all", Name: "全体"},
			{ID: "audio", Name: "音響", VLANs: []int{10}},
			{ID: "video", Name: "映像", VLANs: []int{20, 30}},
			{ID: "lighting", Name: "照明", VLANs: []int{40}},
		},
	}
}

// Broadcaster はライブ配信先（WebSocketハブ）。
type Broadcaster interface {
	Broadcast(msgType string, data any)
	BroadcastSwitch(switchID int64, st *model.SwitchState)
}

// Engine は監視中の全状態を保持する。
type Engine struct {
	mu        sync.RWMutex
	st        *store.Store
	profileID int64
	states    map[int64]*model.SwitchState
	order     []int64
	settings  Settings
	portCfg   map[int64]map[int]store.PortSetting
	aliases   map[string]string // "mac|<MAC>" / "port|<swid>:<ifIndex>" → ラベル
	active    map[string]time.Time // 発報中アラートキー → 最終発報
	baseline  map[string]float64   // SFP Rx 基準（ベースライン比較用）
	hub       Broadcaster
	lastSnap  time.Time
	snapDirty bool
	prevQuerier map[string]string
}

// New を作る。
func New(st *store.Store, profileID int64, hub Broadcaster) *Engine {
	e := &Engine{st: st, profileID: profileID, hub: hub, states: map[int64]*model.SwitchState{},
		portCfg: map[int64]map[int]store.PortSetting{}, aliases: map[string]string{}, active: map[string]time.Time{},
		baseline: map[string]float64{}, prevQuerier: map[string]string{}}
	e.Reload()
	return e
}

// Reload はプロファイル設定・ポート設定・ラベルをDBから再読込する。
func (e *Engine) Reload() {
	set := DefaultSettings()
	if err := e.st.ProfileJSON(e.profileID, "settings_json", &set); err != nil {
		slog.Warn("設定の読込に失敗。既定値を使用", "err", err)
	}
	if set.Thresholds.ErrDelta == 0 && set.Thresholds.CPUPct == 0 {
		set = DefaultSettings()
	}
	sws, _ := e.st.Switches(e.profileID)
	pc := map[int64]map[int]store.PortSetting{}
	for _, s := range sws {
		m, _ := e.st.PortSettings(s.ID)
		pc[s.ID] = m
	}
	al := map[string]string{}
	if list, err := e.st.Aliases(e.profileID); err == nil {
		for _, a := range list {
			al[a.MatchType+"|"+strings.ToUpper(a.MatchKey)] = a.Label
		}
	}
	e.mu.Lock()
	e.settings, e.portCfg, e.aliases = set, pc, al
	keep := map[int64]bool{}
	e.order = e.order[:0]
	for _, s := range sws {
		keep[s.ID] = true
		e.order = append(e.order, s.ID)
		if e.states[s.ID] == nil {
			e.states[s.ID] = &model.SwitchState{ID: s.ID, Name: s.Name, IP: s.IP, GroupTag: s.GroupTag}
		} else {
			e.states[s.ID].Name, e.states[s.ID].GroupTag = s.Name, s.GroupTag
		}
	}
	for id := range e.states {
		if !keep[id] {
			delete(e.states, id)
		}
	}
	e.mu.Unlock()
}

// SetProfile はアクティブプロファイルを切替える。
func (e *Engine) SetProfile(id int64) {
	e.mu.Lock()
	e.profileID = id
	e.states = map[int64]*model.SwitchState{}
	e.active = map[string]time.Time{}
	e.mu.Unlock()
	e.Reload()
}

func (e *Engine) ProfileID() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.profileID
}

func (e *Engine) Settings() Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.settings
}

func (e *Engine) SaveSettings(s Settings) error {
	if err := e.st.SetProfileJSON(e.ProfileID(), "settings_json", s); err != nil {
		return err
	}
	e.mu.Lock()
	e.settings = s
	e.mu.Unlock()
	return nil
}

// Alias はポーラー向けラベル解決（MAC紐付けを優先: 機器を挿し替えても名前が追従する）。
func (e *Engine) Alias(switchID int64, ifIndex int, mac string) string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if l := e.aliases["mac|"+strings.ToUpper(mac)]; l != "" {
		return l
	}
	return e.aliases["port|"+portKey(switchID, ifIndex)]
}

func portKey(sw int64, ix int) string {
	return strings.Join([]string{itoa(int(sw)), itoa(ix)}, ":")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	n := len(b)
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}

// OnState はポーラーからの状態更新（poller.Sink）。
func (e *Engine) OnState(st *model.SwitchState, tier string) {
	now := time.Now().UTC()
	e.mu.Lock()
	cfg := e.portCfg[st.ID]
	prev := e.states[st.ID]
	if prev != nil && st.GroupTag == "" {
		st.GroupTag = prev.GroupTag
	}
	for _, p := range st.Ports {
		if c, ok := cfg[p.IfIndex]; ok {
			p.Excluded, p.ExpectedSpeed, p.AVRole, p.Memo = c.Excluded, c.ExpectedSpeed, c.AVRole, c.Memo
		}
	}
	e.states[st.ID] = st
	set := e.settings
	e.mu.Unlock()

	switch tier {
	case "fast":
		if err := e.st.InsertPortMetrics(now, st.ID, st.Ports); err != nil {
			slog.Warn("ポート時系列の保存に失敗", "switch", st.Name, "err", err)
		}
	case "medium":
		if err := e.st.InsertHealth(now, st); err != nil {
			slog.Warn("ヘルス時系列の保存に失敗", "switch", st.Name, "err", err)
		}
	case "slow":
		e.st.SetSwitchCaps(st.ID, st.Caps, st.Model, st.FW, st.SysObjectID)
		for _, p := range st.Ports {
			for _, d := range p.Devices {
				e.st.UpsertBinding(st.ID, p.IfIndex, d, now)
			}
		}
	}
	if st.Reachable && tier != "error" {
		e.evaluate(st, tier, set)
	}
	if tier == "slow" || tier == "medium" {
		e.evaluateCross(set)
	}
	if e.hub != nil {
		e.hub.BroadcastSwitch(st.ID, st)
	}
}

// OnEvent はイベントを保存・配信する（poller.Sink / Trap受信 / マーカー）。
func (e *Engine) OnEvent(ev model.Event) {
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	ok, err := e.st.InsertEvent(&ev)
	if err != nil {
		slog.Warn("イベント保存に失敗", "err", err, "code", ev.Code)
		return
	}
	if !ok {
		return // 重複（内部ログ再取得など）
	}
	if ev.Code == "DEVICE_GONE" {
		if mac, _ := ev.Detail["mac"].(string); mac != "" {
			e.st.DeactivateBinding(ev.SwitchID, ev.IfIndex, mac)
		}
	}
	if ev.Category != "SYSLOG" {
		e.mu.Lock()
		e.snapDirty = true
		e.mu.Unlock()
	}
	if e.hub != nil {
		e.hub.Broadcast("event_pushed", ev)
	}
}

// raise はアラート条件の立ち上がりで1回だけ発報する（同一キーは条件が消えるまで再発報しない）。
func (e *Engine) raise(seen map[string]bool, key string, ev model.Event) {
	seen[key] = true
	e.mu.Lock()
	_, on := e.active[key]
	if !on {
		e.active[key] = time.Now()
	}
	e.mu.Unlock()
	if !on {
		e.OnEvent(ev)
	}
}

// clearPrefix は今回成立しなかったアラートキーを解除する。
func (e *Engine) clearPrefix(prefix string, seen map[string]bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k := range e.active {
		if strings.HasPrefix(k, prefix) && !seen[k] {
			delete(e.active, k)
		}
	}
}

// States は登録順の状態一覧（コピーではなく参照。呼び出し側で変更しないこと）。
func (e *Engine) States() []*model.SwitchState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]*model.SwitchState, 0, len(e.order))
	for _, id := range e.order {
		if s := e.states[id]; s != nil {
			out = append(out, s)
		}
	}
	return out
}

func (e *Engine) State(id int64) *model.SwitchState {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.states[id]
}

// Run は定期処理（ロールアップ・パージ・スナップショット）を回す。
func (e *Engine) Run(ctx context.Context, ret store.Retention) {
	snapT := time.NewTicker(5 * time.Second)
	rollT := time.NewTicker(time.Minute)
	purgeT := time.NewTicker(10 * time.Minute)
	defer snapT.Stop()
	defer rollT.Stop()
	defer purgeT.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-snapT.C:
			e.mu.Lock()
			due := time.Since(e.lastSnap) >= 30*time.Second || (e.snapDirty && time.Since(e.lastSnap) >= 5*time.Second)
			trig := "periodic"
			if e.snapDirty {
				trig = "event"
			}
			if due {
				e.lastSnap, e.snapDirty = time.Now(), false
			}
			e.mu.Unlock()
			if due && len(e.States()) > 0 {
				if err := e.st.InsertSnapshot(time.Now().UTC(), trig, e.States()); err != nil {
					slog.Warn("スナップショット保存に失敗", "err", err)
				}
			}
		case <-rollT.C:
			if err := e.st.Rollup(ctx, time.Now()); err != nil {
				slog.Warn("1分集約に失敗", "err", err)
			}
		case <-purgeT.C:
			if err := e.st.Purge(ctx, ret, time.Now()); err != nil {
				slog.Warn("古いデータの削除に失敗", "err", err)
			}
		}
	}
}

// Summary は俯瞰カード1枚分。
type Summary struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	IP          string         `json:"ip"`
	GroupTag    string         `json:"group_tag"`
	Reachable   bool           `json:"reachable"`
	LastError   string         `json:"last_error"`
	Model       string         `json:"model"`
	FW          string         `json:"fw"`
	UptimeSec   int64          `json:"uptime_sec"`
	CPU5s       float64        `json:"cpu_5s"`
	MemFreePct  float64        `json:"mem_free_pct"`
	MbufUsedPct float64        `json:"mbuf_used_pct"`
	TempMaxC    float64        `json:"temp_max_c"`
	TempState   string         `json:"temp_state"`
	FansOK      bool           `json:"fans_ok"`
	PSUOK       bool           `json:"psu_ok"`
	PoEBudgetW  float64        `json:"poe_budget_w"`
	PoEUsedW    float64        `json:"poe_used_w"`
	PortsUp     int            `json:"ports_up"`
	PortsTotal  int            `json:"ports_total"`
	RxBps       float64        `json:"rx_bps"`
	TxBps       float64        `json:"tx_bps"`
	Alerts      map[string]int `json:"alerts"`
}

// Overview は俯瞰データ。
type Overview struct {
	Switches     []Summary      `json:"switches"`
	Alerts       map[string]int `json:"alerts"`
	RecentEvents []model.Event  `json:"recent_events"`
}

// ActiveAlerts は発報中アラート数（スイッチID別・重要度別）。
func (e *Engine) activeBySwitch() map[int64]map[string]int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := map[int64]map[string]int{}
	for k := range e.active {
		// キー形式: "<swid>|<sev>|..."
		parts := strings.SplitN(k, "|", 3)
		if len(parts) < 3 {
			continue
		}
		var id int64
		for _, c := range parts[0] {
			id = id*10 + int64(c-'0')
		}
		if out[id] == nil {
			out[id] = map[string]int{}
		}
		out[id][parts[1]]++
	}
	return out
}

// Overview を組み立てる。
func (e *Engine) Overview(withEvents bool) Overview {
	act := e.activeBySwitch()
	ov := Overview{Alerts: map[string]int{model.SevCritical: 0, model.SevWarning: 0, model.SevInfo: 0}}
	for _, s := range e.States() {
		sm := Summary{ID: s.ID, Name: s.Name, IP: s.IP, GroupTag: s.GroupTag, Reachable: s.Reachable, LastError: s.LastError, Model: s.Model, FW: s.FW,
			UptimeSec: s.UptimeSec, CPU5s: s.Health.CPU5s, MemFreePct: s.Health.MemFreePct, MbufUsedPct: s.Health.MbufUsedPct,
			TempMaxC: s.Health.TempMaxC, TempState: s.Health.TempState, FansOK: true, PSUOK: true,
			PoEBudgetW: s.Health.PoEBudgetW, PoEUsedW: s.Health.PoEUsedW, Alerts: map[string]int{model.SevCritical: 0, model.SevWarning: 0}}
		for _, f := range s.Health.Fans {
			if f.State != "operational" && f.State != "notpresent" {
				sm.FansOK = false
			}
		}
		for _, p := range s.Health.PSUs {
			if p.State != "operational" && p.State != "notpresent" {
				sm.PSUOK = false
			}
		}
		for _, p := range s.Ports {
			if p.Type == "lag" {
				continue
			}
			sm.PortsTotal++
			if p.Oper == "up" {
				sm.PortsUp++
			}
			sm.RxBps += p.RxBps
			sm.TxBps += p.TxBps
		}
		for sev, n := range act[s.ID] {
			sm.Alerts[sev] += n
			ov.Alerts[sev] += n
		}
		if !s.Reachable && s.LastPoll.IsZero() == false {
			sm.Alerts[model.SevCritical]++
			ov.Alerts[model.SevCritical]++
		}
		ov.Switches = append(ov.Switches, sm)
	}
	if ov.Switches == nil {
		ov.Switches = []Summary{}
	}
	for sev, n := range act[0] { // 横断アラート（スイッチID=0）
		ov.Alerts[sev] += n
	}
	if withEvents {
		evs, _ := e.st.Events(store.EventFilter{Limit: 30})
		ov.RecentEvents = evs
	}
	return ov
}

// SearchPorts は全台横断ポート検索。
func (e *Engine) SearchPorts(q string, switchID int64, sortKey string) []map[string]any {
	q = strings.ToLower(strings.TrimSpace(q))
	type row struct {
		sw *model.SwitchState
		p  *model.Port
	}
	var rows []row
	for _, s := range e.States() {
		if switchID != 0 && s.ID != switchID {
			continue
		}
		for _, p := range s.Ports {
			if q != "" && !portMatches(p, q) {
				continue
			}
			rows = append(rows, row{s, p})
		}
	}
	key := func(r row) float64 {
		switch sortKey {
		case "err":
			return float64(r.p.ErrDelta + r.p.FCSDelta + r.p.DiscardDelta + r.p.CosDropDelta + r.p.ErrSinceReset)
		case "poe":
			if r.p.PoE != nil {
				return r.p.PoE.PowerW
			}
			return 0
		case "bw":
			return r.p.RxBps + r.p.TxBps
		}
		return 0
	}
	if sortKey != "" {
		sort.SliceStable(rows, func(i, j int) bool { return key(rows[i]) > key(rows[j]) })
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"switch_id": r.sw.ID, "switch_name": r.sw.Name, "port": r.p})
	}
	return out
}

func portMatches(p *model.Port, q string) bool {
	hay := []string{p.Name, p.Alias, p.Memo, p.AVRole, itoa(p.PVID)}
	for _, d := range p.Devices {
		hay = append(hay, d.MAC, d.IP, d.Vendor, d.LLDPName, d.Label, itoa(d.VLAN))
	}
	if q == "anomaly" {
		return p.ErrDelta+p.FCSDelta+p.DiscardDelta+p.CosDropDelta+p.TooLongDelta+p.PauseDelta > 0
	}
	for _, h := range hay {
		if strings.Contains(strings.ToLower(h), q) {
			return true
		}
	}
	return false
}
