package poller

import (
	"sync"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/model"
)

const trapDebounce = 5 * time.Second

// 到達不能中に通知が続いても保留する証拠を有界にする。
const maxPendingLinkPorts = 4096
const maxPendingDownsPerPort = 256

type linkSignals struct {
	downs []*uint64
	down  bool
}

// requests はワーカー毎の追加取得を集約する。SNMP通信中でも受信側を待たせない。
type requests struct {
	mu          sync.Mutex
	wake        chan struct{}
	trap, slow  bool
	lastTrapRun time.Time
	fdbAt       time.Time
	links       map[int]linkSignals
}

func (q *requests) signal() {
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

func (q *requests) addTrap(ev model.Event) {
	q.mu.Lock()
	q.trap = true
	q.slow = q.slow || mib.TrapNeedsSlow(ev.Code)
	if ev.IfIndex > 0 && (ev.Code == "linkDown" || ev.Code == "linkUp") {
		signals, exists := q.links[ev.IfIndex]
		if exists || len(q.links) < maxPendingLinkPorts {
			if q.links == nil {
				q.links = map[int]linkSignals{}
			}
			if ev.Code == "linkDown" && !signals.down && len(signals.downs) < maxPendingDownsPerPort {
				signals.downs = append(signals.downs, ev.TrapUptime)
			}
			signals.down = ev.Code == "linkDown"
			q.links[ev.IfIndex] = signals
		}
	}
	q.mu.Unlock()
	q.signal()
}

func (q *requests) takeTrap(now time.Time) (fast, slow bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.trap || now.Before(q.lastTrapRun.Add(trapDebounce)) {
		return false, false
	}
	fast, slow = true, q.slow
	q.lastTrapRun, q.trap, q.slow = now, false, false
	return fast, slow
}

func (q *requests) takeLinkSignals() map[int]linkSignals {
	q.mu.Lock()
	defer q.mu.Unlock()
	ports := q.links
	q.links = nil
	return ports
}

func (q *requests) restoreLinkSignals(ports map[int]linkSignals) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.links == nil {
		q.links = ports
		return
	}
	for ix, previous := range ports {
		if current, ok := q.links[ix]; ok {
			if len(current.downs) > 0 && len(previous.downs) > 0 && previous.down && sameTrapTicks(previous.downs[len(previous.downs)-1], current.downs[0]) {
				current.downs = current.downs[1:]
			}
			n := min(len(current.downs), maxPendingDownsPerPort-len(previous.downs))
			current.downs = append(previous.downs, current.downs[:n]...)
			q.links[ix] = current
		} else if len(q.links) < maxPendingLinkPorts {
			q.links[ix] = previous
		}
	}
}

func sameTrapTicks(a, b *uint64) bool { return a != nil && b != nil && *a == *b }

func (q *requests) addFDB(at time.Time) {
	q.mu.Lock()
	if q.fdbAt.IsZero() || at.Before(q.fdbAt) {
		q.fdbAt = at
	}
	q.mu.Unlock()
	q.signal()
}

func (q *requests) takeFDB(now time.Time, slow bool) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fdbAt.IsZero() || (!slow && now.Before(q.fdbAt)) {
		return false
	}
	q.fdbAt = time.Time{}
	return !slow
}

func intervalForPhase(base time.Duration, tier, phase string) time.Duration {
	if phase != "setup" && phase != "teardown" {
		return base
	}
	if tier == "slow" {
		return 2 * base
	}
	return 3 * base
}

// TriggerTrap は登録済みで稼働しているスイッチだけを追加取得対象にする。
func (m *Manager) TriggerTrap(ev model.Event) {
	m.mu.Lock()
	w := m.workers[ev.SwitchID]
	m.mu.Unlock()
	if w != nil {
		w.requests.addTrap(ev)
	}
}

// RequestFDB は同じスイッチの追加FDB取得を最も早い期限に集約する。
func (m *Manager) RequestFDB(switchID int64, at time.Time) {
	m.mu.Lock()
	w := m.workers[switchID]
	m.mu.Unlock()
	if w != nil {
		w.requests.addFDB(at)
	}
}
