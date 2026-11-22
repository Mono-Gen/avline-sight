package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

// Alert は発生条件と確認状態を保持する。イベント履歴そのものの編集は行わない。
type Alert struct {
	EventID         int64             `json:"event_id"`
	SwitchID        int64             `json:"switch_id"`
	SwitchName      string            `json:"switch_name"`
	IfIndex         int               `json:"if_index"`
	Severity        string            `json:"severity"`
	Code            string            `json:"code"`
	Title           string            `json:"title"`
	TS              time.Time         `json:"ts"`
	Active          bool              `json:"active"`
	Acked           bool              `json:"acked"`
	ClearedAt       *time.Time        `json:"cleared_at"`
	Port            string            `json:"port"`
	MAC             string            `json:"mac,omitempty"`
	DeviceName      string            `json:"device_name,omitempty"`
	State           string            `json:"state"`
	Suppressed      bool              `json:"suppressed"`
	SuppressedBy    string            `json:"suppressed_by"`
	RawCounters     map[string]string `json:"raw_counters,omitempty"`
	eventSuppressed bool
	priorClearedAt  *time.Time
}

func eventKey(ev model.Event) string {
	return fmt.Sprintf("%d|%s|%s|%d", ev.SwitchID, ev.Severity, ev.Code, ev.IfIndex)
}

func transientKey(ev model.Event) string {
	return eventKey(ev) + "|once|" + strconv.FormatInt(ev.ID, 10)
}

func (e *Engine) recordTransientEvent(ev model.Event) {
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	if ev.Detail == nil {
		ev.Detail = map[string]any{}
	}
	e.mu.Lock()
	a := alertFromEvent(ev)
	suppressed := e.suppressedByLocked(a, time.Now()) != ""
	ev.Detail["_suppressed"], ev.Detail["_one_shot"], ev.Detail["_profile_id"] = suppressed, true, e.profileID
	// 継続条件のない障害は検知時点で完結した発生として、確認まで保持する。
	ev.ClearedAt = &ev.TS
	inserted, err := e.st.InsertEvent(&ev)
	if err != nil {
		e.mu.Unlock()
		slog.Warn("一過性アラートの保存に失敗", "code", ev.Code, "err", err)
		return
	}
	if !inserted {
		e.mu.Unlock()
		return
	}
	if !suppressed {
		e.active[transientKey(ev)] = alertFromEvent(ev)
	}
	e.snapDirty = true
	e.mu.Unlock()
	if e.hub != nil {
		e.hub.Broadcast("event_pushed", ev)
	}
	if !suppressed {
		e.broadcastAlerts()
	}
}

func alertFromEvent(ev model.Event) *Alert {
	a := &Alert{EventID: ev.ID, SwitchID: ev.SwitchID, SwitchName: ev.SwitchName, IfIndex: ev.IfIndex,
		Severity: ev.Severity, Code: ev.Code, Title: ev.Title, TS: ev.TS, Acked: ev.Acked, ClearedAt: ev.ClearedAt}
	a.MAC, _ = ev.Detail["mac"].(string)
	a.MAC = strings.ToUpper(a.MAC)
	a.DeviceName, _ = ev.Detail["name"].(string)
	a.Port, _ = ev.Detail["port"].(string)
	a.eventSuppressed, _ = ev.Detail["_suppressed"].(bool)
	switch raw := ev.Detail["raw_counters"].(type) {
	case map[string]string:
		a.RawCounters = raw
	case map[string]any:
		a.RawCounters = make(map[string]string, len(raw))
		for name, value := range raw {
			if text, ok := value.(string); ok {
				a.RawCounters[name] = text
			}
		}
	}
	return a
}

func sameAlert(a *Alert, ev model.Event) bool {
	b := alertFromEvent(ev)
	if a.Code != b.Code || a.Severity != b.Severity {
		return false
	}
	if deviceAlertCode(a.Code) && a.MAC != "" {
		return a.MAC == b.MAC
	}
	return a.SwitchID == b.SwitchID && a.IfIndex == b.IfIndex && a.MAC == b.MAC
}

func deviceAlertCode(code string) bool {
	return code == "DEVICE_MISSING" || code == "PING_FAIL" || code == "IP_MISMATCH"
}

func (e *Engine) restoreAlerts() {
	pid := e.ProfileID()
	events, err := e.st.Events(store.EventFilter{RestoreProfileID: pid, From: time.Now().Add(-36 * time.Hour)})
	if err != nil {
		slog.Warn("未確認アラートの復元に失敗", "err", err)
		return
	}
	last, err := e.st.LastSeenByMAC(pid, time.Now())
	if err != nil {
		slog.Warn("復元アラートの所在照合に失敗", "err", err)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if pid != e.profileID {
		return
	}
	seen := map[string]bool{}
	for _, ev := range events {
		if recommendationOnly(ev.Code) {
			continue
		}
		a := alertFromEvent(ev)
		// イベント履歴の発生場所を変更せず、移動後のグループで抑止を判定する。
		if deviceAlertCode(a.Code) && a.MAC != "" {
			if ls, ok := last[a.MAC]; ok {
				if a.SwitchID != ls.SwitchID || a.IfIndex != ls.IfIndex {
					a.Port = ""
				}
				a.SwitchID, a.IfIndex = ls.SwitchID, ls.IfIndex
				if s := e.states[a.SwitchID]; s != nil {
					a.SwitchName = s.Name
				}
			}
		}
		identity := fmt.Sprintf("%d|%s|%d|%s", a.SwitchID, a.Code, a.IfIndex, a.MAC)
		if deviceAlertCode(a.Code) && a.MAC != "" {
			identity = deviceKey(a.Code, a.MAC)
		}
		key := eventKey(ev) + "|" + a.MAC + "|restored"
		if a.Code == "LINK_FLAP" {
			key = linkFlapKey(ev)
			identity = key
		} else if oneShot, _ := ev.Detail["_one_shot"].(bool); oneShot {
			key = transientKey(ev)
			identity = key
		}
		if seen[identity] {
			continue
		}
		seen[identity] = true
		if a.eventSuppressed || (a.ClearedAt == nil && e.suppressedByLocked(a, time.Now()) != "") ||
			(isPortMonitoringEvent(a.Code, a.IfIndex) && e.portCfg[a.SwitchID][a.IfIndex].Excluded) {
			continue
		}
		e.active[key] = a
	}
}

func (e *Engine) Alerts() []Alert {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.alertsLocked(time.Now())
}

func (e *Engine) alertsLocked(at time.Time) []Alert {
	out := make([]Alert, 0, len(e.active))
	for _, entry := range e.active {
		a := *entry
		a.State = "latched"
		if a.Active {
			a.State = "active"
		}
		a.SuppressedBy = e.suppressedByLocked(&a, at)
		a.Suppressed = a.SuppressedBy != ""
		if s := e.states[a.SwitchID]; s != nil {
			a.SwitchName = s.Name
			for _, p := range s.Ports {
				if p.IfIndex == a.IfIndex {
					a.Port = p.Name
					break
				}
			}
		}
		out = append(out, a)
	}
	rank := map[string]int{model.SevCritical: 0, model.SevWarning: 1, model.SevInfo: 2}
	sort.Slice(out, func(i, j int) bool {
		if rank[out[i].Severity] != rank[out[j].Severity] {
			return rank[out[i].Severity] < rank[out[j].Severity]
		}
		if !out[i].TS.Equal(out[j].TS) {
			return out[i].TS.After(out[j].TS)
		}
		return out[i].EventID > out[j].EventID
	})
	return out
}

func (e *Engine) broadcastAlerts() {
	if e.hub != nil {
		e.hub.Broadcast("alerts_changed", e.Alerts())
	}
}

func (e *Engine) clearKeys(match func(string, *Alert) bool) {
	e.clearKeysWithPolicy(match, e.dropOnSuppressedClear)
}

func (e *Engine) dropOnSuppressedClear(a *Alert, now time.Time) bool {
	return a.Active && a.priorClearedAt == nil && e.suppressedByLocked(a, now) != ""
}

// 抑止中の再発では、公開済みの復旧記録を抑止解除まで保持する。
func (e *Engine) reactivateAlertLocked(a *Alert, now time.Time) bool {
	a.Active = true
	if a.ClearedAt != nil && e.suppressedByLocked(a, now) != "" {
		a.priorClearedAt, a.ClearedAt = a.ClearedAt, nil
		return false
	}
	a.ClearedAt = nil
	return true
}

func (e *Engine) clearKeysWithPolicy(match func(string, *Alert) bool, discard func(*Alert, time.Time) bool) {
	e.mu.Lock()
	changed := e.clearKeysLocked(match, discard)
	e.mu.Unlock()
	if changed {
		e.broadcastAlerts()
	}
}

func (e *Engine) clearKeysLocked(match func(string, *Alert) bool, discard func(*Alert, time.Time) bool) bool {
	changed := false
	flags := map[string]bool{}
	var clearedIDs []int64
	now := time.Now().UTC()
	for k, a := range e.active {
		if !match(k, a) {
			continue
		}
		remove := discard(a, now)
		if a.Active {
			clearedAt := &now
			if a.priorClearedAt != nil && !remove && e.suppressedByLocked(a, now) != "" {
				clearedAt = a.priorClearedAt
			} else {
				clearedIDs = append(clearedIDs, a.EventID)
			}
			a.Active, a.ClearedAt, a.priorClearedAt = false, clearedAt, nil
			changed = true
		}
		if remove {
			flags[strconv.FormatInt(a.EventID, 10)] = true
		}
		if a.Acked || remove {
			delete(e.active, k)
			changed = true
		}
	}
	e.saveClearedAtLocked(clearedIDs, &now)
	e.saveSuppressionLocked(flags)
	return changed
}

// 復旧・再発生の遷移だけを保存し、再評価のたびに復旧時刻を変えない。
func (e *Engine) saveClearedAtLocked(ids []int64, at *time.Time) {
	if len(ids) == 0 {
		return
	}
	var cleared any
	if at != nil {
		cleared = at.UnixMilli()
	}
	b, err := json.Marshal(ids)
	if err == nil {
		_, err = e.st.DB.Exec(`UPDATE events SET cleared_at=? WHERE id IN (SELECT value FROM json_each(?))`, cleared, string(b))
	}
	if err != nil {
		slog.Warn("アラート復旧時刻の保存に失敗", "err", err)
	}
}

// 抑止中に消えた条件を再起動で再ラッチしないよう、イベントにも抑止状態を残す。
func (e *Engine) saveSuppressionLocked(flags map[string]bool) bool {
	if len(flags) == 0 {
		return true
	}
	b, err := json.Marshal(flags)
	if err == nil {
		_, err = e.st.DB.Exec(`UPDATE events SET detail_json=json_set(
		CASE WHEN json_valid(detail_json) THEN CASE WHEN json_type(detail_json)='object' THEN detail_json ELSE '{}' END ELSE '{}' END,
		'$._suppressed',
		json(CASE WHEN json_extract(?, '$."' || id || '"')=1 THEN 'true' ELSE 'false' END))
		WHERE id IN (SELECT CAST(key AS INTEGER) FROM json_each(?))`, string(b), string(b))
	}
	if err != nil {
		slog.Warn("アラート抑止状態の保存に失敗", "err", err)
	}
	return err == nil
}

func (e *Engine) clearMatching(match func(*Alert) bool) {
	e.clearKeys(func(_ string, a *Alert) bool { return match(a) })
}

func (e *Engine) Ack(id int64, all bool) error {
	e.mu.Lock()
	var err error
	if all {
		err = e.st.AckAll()
	} else {
		err = e.st.Ack(id)
	}
	if err != nil {
		e.mu.Unlock()
		return err
	}
	for k, a := range e.active {
		if all || a.EventID == id {
			a.Acked = true
			if !a.Active {
				delete(e.active, k)
			}
		}
	}
	e.mu.Unlock()
	e.broadcastAlerts()
	return nil
}

var ErrUndoUnavailable = errors.New("alert is unavailable in the current profile")

func (e *Engine) Unack(id int64) error {
	e.mu.Lock()
	events, err := e.st.Events(store.EventFilter{ID: id, ProfileID: e.profileID, Limit: 1})
	if err != nil {
		e.mu.Unlock()
		return err
	}
	if len(events) == 0 || events[0].Category == "TRAP" || (events[0].Severity != model.SevCritical && events[0].Severity != model.SevWarning) {
		e.mu.Unlock()
		return ErrUndoUnavailable
	}
	a := alertFromEvent(events[0])
	if a.eventSuppressed || isPortMonitoringEvent(a.Code, a.IfIndex) && e.portCfg[a.SwitchID][a.IfIndex].Excluded {
		e.mu.Unlock()
		return ErrUndoUnavailable
	}
	if err = e.st.Unack(id); err != nil {
		e.mu.Unlock()
		return err
	}
	found := false
	for _, entry := range e.active {
		if entry.EventID == id {
			entry.Acked = false
			found = true
		}
	}
	if !found {
		a.Acked = false
		key := transientKey(events[0]) + "|restored"
		if oneShot, _ := events[0].Detail["_one_shot"].(bool); oneShot {
			key = transientKey(events[0])
		}
		e.active[key] = a
	}
	e.mu.Unlock()
	e.broadcastAlerts()
	return nil
}

func (e *Engine) reconcileStateAlerts(s *model.SwitchState) {
	changed := false
	var reactivated []int64
	now := time.Now()
	e.mu.Lock()
	for _, a := range e.active {
		if a.SwitchID != s.ID || a.Active {
			continue
		}
		if isPortMonitoringEvent(a.Code, a.IfIndex) && e.portCfg[a.SwitchID][a.IfIndex].Excluded {
			continue
		}
		on := a.Code == "SWITCH_UNREACHABLE" && !s.Reachable && !s.LastPoll.IsZero()
		if a.Code == "LINK_DOWN" && s.Reachable {
			for _, p := range s.Ports {
				if p.IfIndex == a.IfIndex && p.Oper == "down" {
					on = true
				}
			}
		}
		if on {
			if e.reactivateAlertLocked(a, now) {
				reactivated = append(reactivated, a.EventID)
			}
			changed = true
		}
	}
	e.saveClearedAtLocked(reactivated, nil)
	e.mu.Unlock()
	if changed {
		e.broadcastAlerts()
	}
	if !s.Reachable {
		return
	}
	e.clearMatching(func(a *Alert) bool {
		if a.SwitchID != s.ID {
			return false
		}
		if a.Code == "SWITCH_UNREACHABLE" {
			return true
		}
		if a.Code == "LINK_DOWN" {
			for _, p := range s.Ports {
				if p.IfIndex == a.IfIndex {
					return p.Oper == "up"
				}
			}
		}
		return false
	})
}
