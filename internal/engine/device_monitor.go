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
)

// 周期と失敗回数はバックエンド仕様の定期Ping・機器消失条件による。
const periodicPingInterval = 60 * time.Second
const failureThreshold = 2

type Pinger interface {
	One(context.Context, string, time.Duration) ping.Result
}
type systemPinger struct{}

func (systemPinger) One(ctx context.Context, ip string, timeout time.Duration) ping.Result {
	return ping.One(ctx, ip, timeout)
}

type DevicePing struct {
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
	return model.Event{SwitchID: r.SwitchID, SwitchName: r.SwitchName, IfIndex: r.IfIndex, Category: "ANOMALY", Severity: model.SevWarning, Code: code,
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
	last, err := e.st.LastSeenByMAC(pid, time.Now())
	if err != nil {
		slog.Warn("機器の最終観測の読込に失敗", "err", err)
		return
	}
	rows := BuildInventory(e.States(), al, last, InventoryOptions{}, time.Now())
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
	byMAC := map[string]InventoryRow{}
	for _, r := range rows {
		byMAC[r.MAC] = r
	}
	if tier == "slow" || tier == "fdb" || !st.Reachable {
		confirm := false
		valid := st.FDBValid == nil || *st.FDBValid
		present := map[string]bool{}
		for _, p := range st.Ports {
			for _, d := range p.Devices {
				present[strings.ToUpper(d.MAC)] = true
			}
		}
		if st.FDBValid != nil {
			present = map[string]bool{}
			for _, mac := range st.FDBMACs {
				present[strings.ToUpper(mac)] = true
			}
		}
		for mac := range registered {
			ls, seen := last[mac]
			if !seen || ls.SwitchID != st.ID {
				continue
			}
			e.mu.Lock()
			if !st.Reachable || !valid || present[mac] {
				e.missing[mac] = 0
			} else if e.missing[mac] < failureThreshold {
				e.missing[mac]++
			}
			n := e.missing[mac]
			e.mu.Unlock()
			key := deviceKey("DEVICE_MISSING", mac)
			if !st.Reachable || !valid {
				continue
			}
			if n >= failureThreshold {
				e.raise(map[string]bool{}, key, e.deviceEvent("DEVICE_MISSING", byMAC[mac]))
			} else if present[mac] {
				e.clearPrefix(key, map[string]bool{})
			} else if n == 1 {
				confirm = true
			}
		}
		if confirm {
			at := st.LastPoll
			if at.IsZero() {
				at = time.Now()
			}
			e.scheduleFDB(st.ID, at.Add(missingFDBDelay))
		}
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
		if r.Seen && r.SwitchID != last[r.MAC].SwitchID {
			e.clearPrefix(deviceKey("DEVICE_MISSING", r.MAC), map[string]bool{})
		}
	}
	e.clearMatching(func(a *Alert) bool {
		return a.MAC != "" && !registered[a.MAC] && (a.Code == "DEVICE_MISSING" || a.Code == "IP_MISMATCH" || a.Code == "PING_FAIL")
	})
}

func (e *Engine) runPeriodicPing(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
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
			ticker.Reset(interval)
		case <-ticker.C:
			e.periodicPing(ctx)
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
		e.pingResults[r.MAC] = DevicePing{OK: res.Status == ping.StatusOK, RTTms: res.RTTms, TS: now, Source: "periodic", Status: string(res.Status)}
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
