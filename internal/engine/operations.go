package engine

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func (e *Engine) loadOperations() {
	var raw string
	if err := e.st.DB.QueryRow(`SELECT v FROM meta WHERE k='op_phase'`).Scan(&raw); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("運用状態の読込に失敗", "err", err)
		}
		return
	}
	// 旧フェーズは保存し直さず設営として扱い、読取専用セッションでも移行する。
	if raw == "teardown" {
		raw = "setup"
	}
	if validPhase(raw) {
		e.phase = raw
	} else {
		slog.Warn("保存された運用フェーズが不正", "phase", raw)
	}
}

func validPhase(p string) bool {
	return p == "setup" || p == "rehearsal" || p == "show"
}

func (e *Engine) Phase() string { e.mu.RLock(); defer e.mu.RUnlock(); return e.phase }

func (e *Engine) SetPhase(phase string) error {
	if !validPhase(phase) {
		return fmt.Errorf("phase は setup/rehearsal/show です")
	}
	e.operationMu.Lock()
	defer e.operationMu.Unlock()
	e.mu.Lock()
	if e.phase == phase {
		e.mu.Unlock()
		return nil
	}
	var name string
	var err error
	if phase == "show" {
		name = productionBaselineName(time.Now())
		_, err = e.saveBaselineLocked(name, phase)
	} else {
		_, err = e.st.DB.Exec(store.UpdatePhaseSQL, phase)
	}
	if err != nil {
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
	if name != "" {
		e.OnEvent(baselineSavedEvent(name))
	}
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

func (e *Engine) suppressedByLocked(a *Alert, _ time.Time) string {
	device := deviceAlertCode(a.Code)
	if e.phase == "setup" && (a.IfIndex > 0 || device) {
		return "phase"
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
	Name      string  `json:"name"`
	SwitchIDs []int64 `json:"switch_ids"`
}

func (e *Engine) Groups() ([]Group, error) {
	sws, err := e.st.Switches(e.ProfileID())
	if err != nil {
		return nil, err
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	groups := map[string]*Group{}
	for _, s := range sws {
		g := groups[s.GroupTag]
		if g == nil {
			g = &Group{Name: s.GroupTag, SwitchIDs: []int64{}}
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
