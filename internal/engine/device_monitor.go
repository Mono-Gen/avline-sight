package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/ping"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

// 周期と失敗回数はバックエンド仕様の定期Ping・機器消失条件による。
const failureThreshold = 2

type missingCheck struct {
	location  store.LastSeen
	started   time.Time
	count     int
	confirmed bool
	promoted  bool
}

type accessPortSet map[int64]map[int]bool

func accessPorts(states []*model.SwitchState) accessPortSet {
	out := make(accessPortSet, len(states))
	for _, s := range states {
		ports := make(map[int]bool, len(s.Ports))
		for _, p := range s.Ports {
			if !isUplink(s, p) {
				ports[p.IfIndex] = true
			} else {
				ports[p.IfIndex] = false
			}
		}
		out[s.ID] = ports
	}
	return out
}

func (ports accessPortSet) contains(id int64, ix int) bool { return ports[id][ix] }

func (ports accessPortSet) rank(id int64, ix int) int {
	access, known := ports[id][ix]
	if !known {
		return 1
	}
	if access {
		return 2
	}
	return 0
}

type Pinger interface {
	One(context.Context, string, time.Duration) ping.Result
}
type systemPinger struct{}

func (systemPinger) One(ctx context.Context, ip string, timeout time.Duration) ping.Result {
	return ping.One(ctx, ip, timeout)
}

type DevicePing struct {
	IP     string    `json:"-"`
	OK     bool      `json:"ok"`
	RTTms  float64   `json:"rtt_ms"`
	TS     time.Time `json:"ts"`
	Source string    `json:"source"`
	Status string    `json:"-"`
}

func (e *Engine) SetPinger(p Pinger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p != nil {
		e.pinger = p
	}
}

func deviceKey(code, mac string) string { return "device|" + code + "|" + strings.ToUpper(mac) }

func (e *Engine) deviceEvent(code string, r InventoryRow) model.Event {
	return model.Event{SwitchID: r.SwitchID, SwitchName: r.SwitchName, IfIndex: r.IfIndex, Category: "ANOMALY", Severity: model.SevWarning, Code: code, Hint: model.EventHintJa(code),
		Title: fmt.Sprintf("%s: %s (%s)", code, r.Label, r.MAC), Detail: map[string]any{"mac": r.MAC, "name": r.Label, "port": r.Port, "last_ip": r.IP, "planned_ip": r.PlannedIP}}
}

func (e *Engine) evaluateDevices(st *model.SwitchState, tier string) {
	e.deviceMu.Lock()
	defer e.deviceMu.Unlock()
	pid := e.ProfileID()
	al, err := e.st.Aliases(pid)
	if err != nil {
		slog.Warn("登録機器の読込に失敗", "err", err)
		return
	}
	now := time.Now()
	states := e.States()
	checkMissing := tier == "slow" || tier == "fdb" || !st.Reachable
	last, err := e.st.LastSeenByMAC(pid, now)
	if err != nil {
		slog.Warn("機器の最終観測の読込に失敗", "err", err)
		return
	}
	rows := BuildInventory(states, al, last, InventoryOptions{}, now)
	var access accessPortSet
	var locations map[string]store.LastSeen
	if checkMissing {
		access = accessPorts(states)
		locations, err = e.st.LastSeenByMACPreferringPorts(pid, now, access.rank)
		if err != nil {
			slog.Warn("機器の最終所在の読込に失敗", "err", err)
			return
		}
	}
	registered := map[string]bool{}
	for _, a := range al {
		if a.MatchType == "mac" {
			registered[strings.ToUpper(a.MatchKey)] = true
		}
	}
	e.mu.Lock()
	for mac := range e.missing {
		if !registered[mac] {
			delete(e.missing, mac)
		}
	}
	for mac := range e.pingFails {
		if !registered[mac] {
			delete(e.pingFails, mac)
		}
	}
	e.mu.Unlock()
	if checkMissing {
		e.evaluateMissingDevices(st, states, access, locations, rows, registered, now)
	}
	phase := e.Phase()
	for _, r := range rows {
		if !registered[r.MAC] {
			continue
		}
		key := deviceKey("IP_MISMATCH", r.MAC)
		if (phase == "rehearsal" || phase == "show") && r.IPMismatch {
			e.raise(map[string]bool{}, key, e.deviceEvent("IP_MISMATCH", r))
		} else {
			e.clearPrefix(key, map[string]bool{})
		}
	}
	e.clearMatching(func(a *Alert) bool {
		return a.MAC != "" && !registered[a.MAC] && (a.Code == "DEVICE_MISSING" || a.Code == "IP_MISMATCH" || a.Code == "PING_FAIL")
	})
}

func (e *Engine) evaluateMissingDevices(st *model.SwitchState, states []*model.SwitchState, access accessPortSet,
	last map[string]store.LastSeen, rows []InventoryRow, registered map[string]bool, now time.Time) {
	// FDB単独取得では統合済み機器一覧が更新されないため、取得した行のポートを使う。
	observed := map[string]map[int64]store.LastSeen{}
	for _, s := range states {
		if !s.Reachable || s.FDBValid == nil || !*s.FDBValid || s.FDBLastOK.IsZero() {
			continue
		}
		for ix, macs := range s.FDBPorts {
			if !access.contains(s.ID, ix) {
				continue
			}
			for _, rawMAC := range macs {
				mac := strings.ToUpper(rawMAC)
				if !registered[mac] {
					continue
				}
				if observed[mac] == nil {
					observed[mac] = map[int64]store.LastSeen{}
				}
				if old, seen := observed[mac][s.ID]; !seen || ix < old.IfIndex {
					observed[mac][s.ID] = store.LastSeen{SwitchID: s.ID, IfIndex: ix, TS: s.FDBLastOK}
				}
			}
		}
	}
	present := map[string]bool{}
	if st.FDBValid == nil {
		for _, p := range st.Ports {
			for _, d := range p.Devices {
				present[strings.ToUpper(d.MAC)] = true
			}
		}
	} else {
		for _, mac := range st.FDBMACs {
			present[strings.ToUpper(mac)] = true
		}
	}
	ok := st.Reachable && (st.FDBValid == nil || *st.FDBValid)
	pollAt := st.LastPoll
	if pollAt.IsZero() {
		pollAt = now
	}
	fdbAt := st.FDBLastOK
	if fdbAt.IsZero() {
		fdbAt = pollAt
	}
	confirm := false
	for _, row := range rows {
		mac := row.MAC
		if !registered[mac] {
			continue
		}
		e.mu.Lock()
		check := e.missing[mac]
		ls := last[mac]
		if check.promoted && check.started.IsZero() {
			seen := observed[mac][ls.SwitchID]
			if ls.SwitchID == check.location.SwitchID && ls.IfIndex == check.location.IfIndex ||
				seen.IfIndex == ls.IfIndex && seen.TS.After(check.location.TS) {
				delete(e.missing, mac)
				check = missingCheck{}
			}
		}
		if check.location.SwitchID == 0 {
			check.location = ls
		}
		id := check.location.SwitchID
		if id != 0 && access.rank(id, check.location.IfIndex) == 0 {
			delete(e.missing, mac)
			e.mu.Unlock()
			e.clearDeviceMissing(mac, false)
			continue
		}
		var moved store.LastSeen
		if !check.started.IsZero() {
			for other, seen := range observed[mac] {
				if other != id && seen.TS.After(check.started) &&
					(seen.TS.After(moved.TS) || (seen.TS.Equal(moved.TS) && other < moved.SwitchID)) {
					moved = seen
				}
			}
		}
		elsewhere := moved.SwitchID != 0
		if elsewhere || (id == st.ID && ok && present[mac]) {
			switch {
			case elsewhere:
				// 単独取得は接続履歴を更新しないため、次の判定とリンク断に移動先を引き継ぐ。
				e.missing[mac] = missingCheck{location: moved, promoted: true}
			case check.promoted && (ls.SwitchID != id || ls.IfIndex != check.location.IfIndex):
				e.missing[mac] = missingCheck{location: check.location, promoted: true}
			default:
				delete(e.missing, mac)
			}
			e.mu.Unlock()
			e.clearDeviceMissing(mac, elsewhere)
			continue
		}
		if id != st.ID {
			e.mu.Unlock()
			continue
		}
		if !ok {
			if !check.confirmed {
				if check.promoted {
					e.missing[mac] = missingCheck{location: check.location, promoted: true}
				} else {
					delete(e.missing, mac)
				}
			} else {
				check.count = 0
				e.missing[mac] = check
			}
			e.mu.Unlock()
			continue
		}
		if check.started.IsZero() {
			check.started = fdbAt
		}
		if check.count < failureThreshold {
			check.count++
		}
		check.confirmed = check.confirmed || check.count >= failureThreshold
		e.missing[mac] = check
		e.mu.Unlock()
		if check.count == 1 {
			confirm = true
		}
		if check.count >= failureThreshold {
			row.SwitchID, row.SwitchName, row.IfIndex, row.Port = id, st.Name, check.location.IfIndex, ""
			for _, p := range st.Ports {
				if p.IfIndex == row.IfIndex {
					row.Port = p.Name
					break
				}
			}
			e.raise(map[string]bool{}, deviceKey("DEVICE_MISSING", mac), e.deviceEvent("DEVICE_MISSING", row))
		}
	}
	if confirm {
		e.scheduleFDB(st.ID, pollAt.Add(missingFDBDelay))
	}
}

func (e *Engine) clearDeviceMissing(mac string, elsewhere bool) {
	match := func(_ string, a *Alert) bool { return a.Code == "DEVICE_MISSING" && a.MAC == mac }
	if !elsewhere {
		e.clearKeys(match)
		return
	}
	// 別のアクセスポートで復旧を確認した場合も、表示済みの未確認ラッチは保持する。
	e.clearKeysWithPolicy(match, func(a *Alert, _ time.Time) bool { return a.Active && a.eventSuppressed })
}

func (e *Engine) runPeriodicPing(ctx context.Context, interval func() time.Duration) {
	ticker := time.NewTicker(interval())
	defer ticker.Stop()
	defer func() {
		e.mu.Lock()
		if e.pingCancel != nil {
			e.pingCancel()
			e.pingCancel = nil
		}
		e.mu.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-e.phaseWake:
			ticker.Reset(interval())
		case <-ticker.C:
			e.periodicPing(ctx)
			ticker.Reset(interval())
		}
	}
}

func (e *Engine) periodicPing(ctx context.Context) {
	e.mu.Lock()
	if e.phase != "setup" && e.phase != "rehearsal" {
		e.mu.Unlock()
		return
	}
	batchCtx, cancel := context.WithCancel(ctx)
	e.pingCancel = cancel
	pid, pinger := e.profileID, e.pinger
	e.mu.Unlock()
	defer cancel()
	al, err := e.st.Aliases(pid)
	if err != nil {
		slog.Warn("定期Ping対象の読込に失敗", "err", err)
		return
	}
	last, err := e.st.LastSeenByMAC(pid, time.Now())
	if err != nil {
		slog.Warn("定期Ping宛先の読込に失敗", "err", err)
		return
	}
	registered := map[string]bool{}
	for _, a := range al {
		if a.MatchType == "mac" {
			registered[strings.ToUpper(a.MatchKey)] = true
		}
	}
	rows := BuildInventory(e.States(), al, last, InventoryOptions{}, time.Now())
	targets := make([]InventoryRow, 0, len(al))
	for _, r := range rows {
		if registered[r.MAC] {
			if r.IP == "" {
				r.IP = r.PlannedIP
			}
			if r.IP != "" {
				targets = append(targets, r)
			}
		}
	}
	results := make([]ping.Result, len(targets))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < ping.MaxParallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if batchCtx.Err() != nil {
					continue
				}
				results[i] = pinger.One(batchCtx, targets[i].IP, ping.DefaultTimeout)
			}
		}()
	}
send:
	for i := range targets {
		select {
		case <-batchCtx.Done():
			break send
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	e.operationMu.Lock()
	defer e.operationMu.Unlock()
	e.deviceMu.Lock()
	defer e.deviceMu.Unlock()
	if batchCtx.Err() != nil || e.ProfileID() != pid {
		return
	}
	for i, r := range targets {
		res := results[i]
		now := time.Now().UTC()
		e.mu.Lock()
		e.pingResults[r.MAC] = DevicePing{IP: r.IP, OK: res.Status == ping.StatusOK, RTTms: res.RTTms, TS: now, Source: "periodic", Status: string(res.Status)}
		if res.Status == ping.StatusOK {
			e.pingFails[r.MAC] = 0
		} else if e.pingFails[r.MAC] < failureThreshold {
			e.pingFails[r.MAC]++
		}
		n := e.pingFails[r.MAC]
		e.mu.Unlock()
		key := deviceKey("PING_FAIL", r.MAC)
		if n >= failureThreshold {
			e.raise(map[string]bool{}, key, e.deviceEvent("PING_FAIL", r))
		} else if res.Status == ping.StatusOK {
			e.clearPrefix(key, map[string]bool{})
		}
	}
}

// 古いプロファイルの結果が切替後へ流れ込まないよう、手動結果も切替と直列化する。
func (e *Engine) RecordManualPing(profileID int64, results map[string]PingMark) {
	e.deviceMu.Lock()
	defer e.deviceMu.Unlock()
	if e.ProfileID() == profileID {
		e.RecordPing(results)
	}
}
