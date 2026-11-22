package engine

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// 追加取得は、連続したFDB不在を次のSlow周期より早く確認するために使う。
const missingFDBDelay = 10 * time.Second
const linkFlapWindow = 60 * time.Second

func linkFlapKey(ev model.Event) string {
	return eventKey(ev) + "|flap|" + strconv.FormatInt(ev.ID, 10)
}

func (e *Engine) SetFDBRequester(fn func(int64, time.Time)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requestFDB = fn
}

func (e *Engine) scheduleFDB(id int64, at time.Time) {
	e.mu.RLock()
	request := e.requestFDB
	e.mu.RUnlock()
	if request != nil {
		request(id, at)
	}
}

func (e *Engine) requestFDBForLinkDown(id int64, ifIndex int) {
	e.mu.RLock()
	pid, request := e.profileID, e.requestFDB
	e.mu.RUnlock()
	if request == nil || ifIndex <= 0 {
		return
	}
	last, err := e.st.LastSeenByMAC(pid, time.Now())
	if err != nil {
		slog.Warn("リンク断時の機器の最終所在を取得できません", "err", err)
		return
	}
	e.mu.RLock()
	for mac, ls := range last {
		if ls.SwitchID == id && ls.IfIndex == ifIndex && e.aliases["mac|"+mac] != "" {
			e.mu.RUnlock()
			request(id, time.Now())
			return
		}
	}
	e.mu.RUnlock()
}

func (e *Engine) recordTrap(ev model.Event) {
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	if err := e.st.RecordTrap(ev.SwitchID, ev.TS); err != nil {
		slog.Warn("最終Trap受信時刻の保存に失敗", "switch_id", ev.SwitchID, "err", err)
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, known := e.switchGroups[ev.SwitchID]; known && ev.TS.After(e.lastTraps[ev.SwitchID]) {
		e.lastTraps[ev.SwitchID] = time.UnixMilli(ev.TS.UnixMilli()).UTC()
	}
}

func (e *Engine) recordLinkFlap(ev model.Event) {
	if ev.IfIndex <= 0 || ev.SwitchID == 0 {
		return
	}
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	count, _ := ev.Detail["count"].(int)
	if count < 1 {
		count = 1
	}
	e.mu.Lock()
	a := alertFromEvent(ev)
	suppressed := e.suppressedByLocked(a, time.Now()) != ""
	counted, err := e.st.CountRecentLinkFlap(ev.SwitchID, ev.IfIndex, count, ev.TS.Add(-linkFlapWindow), ev.TS, suppressed)
	if err != nil {
		e.mu.Unlock()
		slog.Warn("リンクフラップの件数更新に失敗", "err", err)
		return
	}
	if counted {
		e.mu.Unlock()
		return
	}
	if ev.Detail == nil {
		ev.Detail = map[string]any{}
	}
	ev.Detail["count"] = count
	ev.Detail["_suppressed"] = suppressed
	ev.ClearedAt = &ev.TS
	inserted, err := e.st.InsertEvent(&ev)
	if err != nil {
		e.mu.Unlock()
		slog.Warn("リンクフラップの保存に失敗", "err", err)
		return
	}
	if !inserted {
		e.mu.Unlock()
		return
	}
	if !suppressed {
		a.EventID, a.ClearedAt = ev.ID, &ev.TS
		// 60秒を超える独立したフラップでは、前の未確認エントリに新しい発生を上書きしない。
		e.active[linkFlapKey(ev)] = a
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
