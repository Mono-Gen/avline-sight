package engine

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func (e *Engine) loadOperations() {
	for _, key := range []string{"op_phase", "work_mode"} {
		var raw string
		if err := e.st.DB.QueryRow(`SELECT v FROM meta WHERE k=?`, key).Scan(&raw); err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				slog.Warn("運用状態の読込に失敗", "key", key, "err", err)
			}
			continue
		}
		if key == "op_phase" {
			if validPhase(raw) {
				e.phase = raw
			} else {
				slog.Warn("保存された運用フェーズが不正", "phase", raw)
			}
		} else if err := json.Unmarshal([]byte(raw), &e.work); err != nil {
			slog.Warn("作業状態の読込に失敗", "err", err)
			e.work = map[string]time.Time{}
		}
	}
	if e.work == nil {
		e.work = map[string]time.Time{}
	}
	if err := e.ExpireWork(time.Now().UTC()); err != nil {
		slog.Warn("作業期限の解除に失敗", "err", err)
	}
}

func validPhase(p string) bool {
	return p == "setup" || p == "rehearsal" || p == "show" || p == "teardown"
}

func (e *Engine) Phase() string { e.mu.RLock(); defer e.mu.RUnlock(); return e.phase }

func (e *Engine) SetPhase(phase string) error {
	if !validPhase(phase) {
		return fmt.Errorf("phase は setup/rehearsal/show/teardown です")
	}
	e.operationMu.Lock()
	defer e.operationMu.Unlock()
	e.mu.Lock()
	if e.phase == phase {
		e.mu.Unlock()
		return nil
	}
	if _, err := e.st.DB.Exec(`INSERT INTO meta(k,v) VALUES('op_phase',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, phase); err != nil {
		e.mu.Unlock()
		return err
	}
	e.phase = phase
	if e.pingCancel != nil {
		e.pingCancel()
	}
	e.pingFails = map[string]int{}
	e.exposeActiveAlertsLocked()
	e.mu.Unlock()
	select {
	case e.phaseWake <- struct{}{}:
	default:
	}
	e.OnEvent(model.Event{Category: "SYSTEM", Severity: model.SevInfo, Code: "PHASE_CHANGED", Title: "運用フェーズを変更: " + phase, Detail: map[string]any{"phase": phase}})
	if e.hub != nil {
		e.hub.Broadcast("phase_changed", map[string]string{"phase": phase})
	}
	e.broadcastAlerts()
	return nil
}

func (e *Engine) suppressedByLocked(a *Alert, now time.Time) string {
	device := deviceAlertCode(a.Code)
	if e.phase == "teardown" || (e.phase == "setup" && (a.IfIndex > 0 || device)) {
		return "phase"
	}
	if a.SwitchID != 0 {
		if until := e.work["*"]; until.After(now) {
			return "work"
		}
		if group, ok := e.switchGroups[a.SwitchID]; ok && e.work[group].After(now) {
			return "work"
		}
	}
	return ""
}

func (e *Engine) exposeActiveAlertsLocked() {
	flags := map[string]bool{}
	var exposed []*Alert
	var reactivated []int64
	now := time.Now()
	for _, a := range e.active {
		if !a.Active || e.suppressedByLocked(a, now) != "" {
			continue
		}
		if a.eventSuppressed {
			flags[strconv.FormatInt(a.EventID, 10)] = false
			exposed = append(exposed, a)
		}
		if a.priorClearedAt != nil {
			reactivated = append(reactivated, a.EventID)
			a.priorClearedAt = nil
		}
	}
	e.saveClearedAtLocked(reactivated, nil)
	if e.saveSuppressionLocked(flags) {
		for _, a := range exposed {
			a.eventSuppressed = false
		}
	}
}

type Group struct {
	Name      string     `json:"name"`
	SwitchIDs []int64    `json:"switch_ids"`
	WorkUntil *time.Time `json:"work_until"`
}

func (e *Engine) Groups() ([]Group, error) {
	sws, err := e.st.Switches(e.ProfileID())
	if err != nil {
		return nil, err
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	groups := map[string]*Group{}
	now := time.Now()
	for _, s := range sws {
		g := groups[s.GroupTag]
		if g == nil {
			g = &Group{Name: s.GroupTag, SwitchIDs: []int64{}}
			until := e.work[s.GroupTag]
			if e.work["*"].After(until) {
				until = e.work["*"]
			}
			if until.After(now) {
				g.WorkUntil = &until
			}
			groups[s.GroupTag] = g
		}
		g.SwitchIDs = append(g.SwitchIDs, s.ID)
	}
	out := make([]Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (e *Engine) SetWork(group string, minutes int) error {
	if minutes != 15 && minutes != 30 && minutes != 60 && minutes != 120 {
		return fmt.Errorf("minutes は 15/30/60/120 です")
	}
	return e.changeWork(group, time.Now().UTC().Add(time.Duration(minutes)*time.Minute), false)
}

func (e *Engine) EndWork(group string) error { return e.changeWork(group, time.Time{}, true) }

func (e *Engine) changeWork(group string, until time.Time, remove bool) error {
	e.operationMu.Lock()
	defer e.operationMu.Unlock()
	e.mu.Lock()
	next := make(map[string]time.Time, len(e.work)+1)
	for k, v := range e.work {
		next[k] = v
	}
	if remove {
		if _, ok := next[group]; !ok {
			e.mu.Unlock()
			return nil
		}
		delete(next, group)
	} else {
		next[group] = until
	}
	if err := e.saveWorkLocked(next); err != nil {
		e.mu.Unlock()
		return err
	}
	e.exposeActiveAlertsLocked()
	e.mu.Unlock()
	code, title := "WORK_STARTED", "作業中を開始: "
	if remove {
		code, title = "WORK_ENDED", "作業中を解除: "
	}
	e.OnEvent(model.Event{Category: "SYSTEM", Severity: model.SevInfo, Code: code, Title: title + group, Detail: map[string]any{"group": group, "until": until}})
	e.broadcastWork()
	return nil
}

func (e *Engine) saveWorkLocked(next map[string]time.Time) error {
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if _, err = e.st.DB.Exec(`INSERT INTO meta(k,v) VALUES('work_mode',?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, string(b)); err != nil {
		return err
	}
	e.work = next
	return nil
}

func (e *Engine) ExpireWork(now time.Time) error {
	e.operationMu.Lock()
	defer e.operationMu.Unlock()
	e.mu.Lock()
	next := map[string]time.Time{}
	var expired []string
	for k, v := range e.work {
		if !v.After(now) {
			expired = append(expired, k)
		} else {
			next[k] = v
		}
	}
	if len(expired) == 0 {
		e.mu.Unlock()
		return nil
	}
	if err := e.saveWorkLocked(next); err != nil {
		e.mu.Unlock()
		return err
	}
	e.exposeActiveAlertsLocked()
	e.mu.Unlock()
	for _, g := range expired {
		e.OnEvent(model.Event{Category: "SYSTEM", Severity: model.SevInfo, Code: "WORK_ENDED", Title: "作業期限が終了: " + g, Detail: map[string]any{"group": g}})
	}
	e.broadcastWork()
	return nil
}

func (e *Engine) broadcastWork() {
	if e.hub != nil {
		groups, err := e.Groups()
		if err != nil {
			slog.Warn("グループ状態の配信に失敗", "err", err)
		} else {
			e.hub.Broadcast("work_changed", groups)
		}
	}
	e.broadcastAlerts()
}
