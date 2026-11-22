package engine

import (
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
	"testing"
	"time"
)

func TestLinkFlapOneShotDedupeAndSuppression(t *testing.T) {
	e, st := newTestEngine(t)
	now := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Millisecond)
	flap := func(ix int, at time.Time) { ev := alertEvent("LINK_FLAP", ix); ev.TS = at; e.OnEvent(ev) }
	flap(1, now)
	counted := alertEvent("LINK_FLAP", 1)
	counted.TS = now.Add(59 * time.Second)
	counted.Detail = map[string]any{"count": 2}
	e.OnEvent(counted)
	flap(2, now.Add(59*time.Second))
	a := e.Alerts()
	if len(a) != 2 {
		t.Fatalf("60秒以内の重複: %+v", a)
	}
	for _, item := range a {
		if item.Active || item.Acked || item.State != "latched" || item.ClearedAt == nil {
			t.Fatalf("即時ラッチ: %+v", item)
		}
	}
	list, err := st.Events(store.EventFilter{Limit: 20})
	if err != nil || len(list) != 2 {
		t.Fatalf("イベント履歴: %+v %v", list, err)
	}
	for _, item := range list {
		if item.IfIndex == 1 && item.Detail["count"] != float64(3) {
			t.Fatalf("累積回数: %+v", item)
		}
	}
	flap(1, now.Add(time.Minute))
	if len(e.Alerts()) != 3 {
		t.Fatal("60秒後のフラップが新規記録されない")
	}
	e = New(st, e.ProfileID(), nil)
	if len(e.Alerts()) != 3 {
		t.Fatal("未確認フラップが再起動で失われた")
	}
	if err := e.Ack(0, true); err != nil || len(e.Alerts()) != 0 {
		t.Fatalf("確認で削除されない: %v %+v", err, e.Alerts())
	}
	if err := e.SetPhase("setup"); err != nil {
		t.Fatal(err)
	}
	flap(3, time.Now())
	if err := e.SetPhase("show"); err != nil {
		t.Fatal(err)
	}
	if err := e.SetWork("*", 15); err != nil {
		t.Fatal(err)
	}
	flap(4, time.Now())
	if len(e.Alerts()) != 0 || codes(t, st)["LINK_FLAP"] != 5 {
		t.Fatalf("抑止中は履歴にのみ記録: %+v", e.Alerts())
	}
}

func TestTrapTimestampAndNoAlertOnRestart(t *testing.T) {
	e, st := newTestEngine(t)
	ts := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	ev := model.Event{SwitchID: 1, Category: "TRAP", Severity: model.SevWarning, Code: "linkDown", IfIndex: 3, TS: ts}
	e.OnEvent(ev)
	ev.TS = ts.Add(-time.Second)
	e.OnEvent(ev)
	assertTime := func(e *Engine) {
		t.Helper()
		s := e.Overview(false).Switches
		if len(s) != 1 || s[0].LastTrapTS == nil || !s[0].LastTrapTS.Equal(ts) {
			t.Fatalf("最終Trap時刻: %+v", s)
		}
		if len(e.Alerts()) != 0 {
			t.Fatal("Trapだけでラッチされた")
		}
	}
	assertTime(e)
	assertTime(New(st, e.ProfileID(), nil))
	sws, err := st.Switches(e.ProfileID())
	if err != nil || len(sws) != 1 || sws[0].LastTrapTS == nil || !sws[0].LastTrapTS.Equal(ts) {
		t.Fatalf("永続化: %+v %v", sws, err)
	}
}

func TestFDBConfirmationAfterFirstMissAndLinkDown(t *testing.T) {
	e, st := newTestEngine(t)
	mac01, mac02 := "00:11:22:33:44:55", "00:11:22:33:44:66"
	registerDevice(t, e, st, mac01, "")
	registerDevice(t, e, st, mac02, "")
	type request struct {
		id int64
		at time.Time
	}
	var requested []request
	e.SetFDBRequester(func(id int64, at time.Time) { requested = append(requested, request{id, at}) })
	s := &model.SwitchState{ID: 1, Name: "SW", Reachable: true, Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Oper: "up", Devices: []model.Device{{MAC: mac01}, {MAC: mac02}}}}}
	e.OnState(s, "slow")
	before := time.Now()
	e.OnEvent(alertEvent("LINK_DOWN", 2))
	if len(requested) != 0 {
		t.Fatal("登録機器のいないポートで追加取得")
	}
	e.OnEvent(alertEvent("LINK_DOWN", 1))
	if len(requested) != 1 || requested[0].id != 1 || requested[0].at.Before(before) || requested[0].at.After(time.Now()) {
		t.Fatalf("リンク断の即時取得: %+v", requested)
	}
	requested = nil
	s.Ports[0].Devices = nil
	s.LastPoll = time.Now().Add(-time.Second)
	e.OnState(s, "slow")
	if len(requested) != 1 || requested[0].id != 1 || !requested[0].at.Equal(s.LastPoll.Add(10*time.Second)) {
		t.Fatalf("同じスイッチで追加は10秒後に1回: %+v", requested)
	}
	if codes(t, st)["DEVICE_MISSING"] != 0 {
		t.Fatal("1回不在で発報")
	}
	e.OnState(s, "fast")
	if codes(t, st)["DEVICE_MISSING"] != 0 {
		t.Fatal("Fastの古いFDBを2回目に使用")
	}
	s.LastPoll = requested[0].at
	e.OnState(s, "fdb")
	if codes(t, st)["DEVICE_MISSING"] != 2 || len(requested) != 1 {
		t.Fatal("FDB単独取得で2件の消失を確認して終了")
	}
}
