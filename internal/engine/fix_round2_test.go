package engine

import (
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func TestAcknowledgedLinkFlapRecursInsideWindow(t *testing.T) {
	e, st := newTestEngine(t)
	ev := alertEvent("LINK_FLAP", 1)
	ev.TS = time.Now().UTC().Add(-time.Minute)
	e.OnEvent(ev)
	oldID := e.Alerts()[0].EventID
	if err := e.Ack(oldID, false); err != nil {
		t.Fatal(err)
	}
	ev.TS = ev.TS.Add(5 * time.Second)
	e.OnEvent(ev)
	if alerts := e.Alerts(); len(alerts) != 1 || alerts[0].EventID == oldID || alerts[0].Acked {
		t.Fatalf("再発のラッチ: %+v", alerts)
	}
	events, err := st.Events(store.EventFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].ID != oldID || events[1].Detail["count"] != float64(1) {
		t.Fatalf("確認済みイベントが変更された: %+v", events)
	}
}

func TestRegisteredSwitchNameSurvivesStalePoll(t *testing.T) {
	e, st := newTestEngine(t)
	switches, err := st.Switches(e.ProfileID())
	if err != nil || len(switches) != 1 {
		t.Fatalf("スイッチ一覧: %+v (%v)", switches, err)
	}
	cfg := switches[0]
	oldName := cfg.Name
	cfg.Name = "Renamed"
	if err := st.SaveSwitch(&cfg); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.OnState(&model.SwitchState{ID: cfg.ID, Name: oldName, Reachable: true}, "fast")
	if got := e.State(cfg.ID).Name; got != cfg.Name {
		t.Fatalf("ポーリング後の名前 = %q, want %q", got, cfg.Name)
	}
}

func TestRemovedSwitchClearsActiveAlerts(t *testing.T) {
	for _, remove := range []string{"disable", "disable in setup", "delete"} {
		t.Run(remove, func(t *testing.T) {
			e, st := newTestEngine(t)
			switches, err := st.Switches(e.ProfileID())
			if err != nil || len(switches) != 1 {
				t.Fatalf("スイッチ一覧: %+v (%v)", switches, err)
			}
			cfg := switches[0]
			for _, ix := range []int{1, 2} {
				e.OnEvent(alertEvent("LINK_DOWN", ix))
			}
			if err := e.Ack(e.Alerts()[0].EventID, false); err != nil {
				t.Fatal(err)
			}
			if remove == "disable in setup" {
				if err := e.SetPhase("setup"); err != nil {
					t.Fatal(err)
				}
			}
			if remove == "delete" {
				err = st.DeleteSwitch(cfg.ID)
			} else {
				cfg.Enabled = false
				err = st.SaveSwitch(&cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			e.Reload()
			e.OnEvent(alertEvent("LINK_DOWN", 3))
			e.OnState(&model.SwitchState{ID: cfg.ID, Reachable: true}, "fast")
			if e.State(cfg.ID) != nil {
				t.Fatal("監視から外したスイッチの状態が残った")
			}
			want := 1
			if remove == "delete" {
				want = 0
			}
			for _, engine := range []*Engine{e, New(st, e.ProfileID(), nil)} {
				alerts := engine.Alerts()
				if len(alerts) != want || (want == 1 && (alerts[0].Active || alerts[0].Acked || alerts[0].ClearedAt == nil)) {
					t.Fatalf("監視終了後のラッチ: %+v", alerts)
				}
			}
			events, err := st.Events(store.EventFilter{Q: "検証"})
			if err != nil || len(events) != 2 || events[0].ClearedAt == nil || events[1].ClearedAt == nil {
				t.Fatalf("監視終了後の履歴: %+v (%v)", events, err)
			}
		})
	}
}

func TestSuppressionPreservesRecoveredUnackedLatch(t *testing.T) {
	e, st := newTestEngine(t)
	e.OnEvent(alertEvent("LINK_DOWN", 1))
	linkUp := alertEvent("LINK_UP", 1)
	linkUp.Severity = model.SevInfo
	e.OnEvent(linkUp)
	original := e.Alerts()[0]
	if err := e.SetPhase("setup"); err != nil {
		t.Fatal(err)
	}
	e.OnEvent(alertEvent("LINK_DOWN", 1))
	e.OnState(&model.SwitchState{ID: 1, Reachable: true, Ports: []*model.Port{{IfIndex: 1, Oper: "down"}}}, "fast")
	if a := New(st, e.ProfileID(), nil).Alerts(); len(a) != 1 || a[0].EventID != original.EventID ||
		a[0].Active || !a[0].Suppressed || a[0].ClearedAt == nil {
		t.Fatalf("抑止中の再発で既存ラッチがDBから失われた: %+v", a)
	}
	e.OnEvent(linkUp)
	for _, current := range []*Engine{e, New(st, e.ProfileID(), nil)} {
		a := current.Alerts()
		if len(a) != 1 || a[0].EventID != original.EventID || a[0].Active || !a[0].Suppressed ||
			current.Overview(false).Alerts[model.SevWarning] != 0 {
			t.Fatalf("抑止中の復旧済みラッチ: %+v", a)
		}
		if err := current.SetPhase("show"); err != nil {
			t.Fatal(err)
		}
		a = current.Alerts()
		if len(a) != 1 || a[0].EventID != original.EventID || a[0].Suppressed ||
			current.Overview(false).Alerts[model.SevWarning] != 1 {
			t.Fatalf("抑止解除後の復旧済みラッチ: %+v", a)
		}
	}
}

func TestAlertClearPersistsOnlyTransitionsAndNormalizesLegacyJSON(t *testing.T) {
	e, st := newTestEngine(t)
	e.OnEvent(alertEvent("LINK_DOWN", 1))
	linkUp := alertEvent("LINK_UP", 1)
	linkUp.Severity = model.SevInfo
	e.OnEvent(linkUp)
	if _, err := st.DB.Exec(`CREATE TABLE event_write_count (id INTEGER);
		CREATE TRIGGER count_event_updates AFTER UPDATE ON events BEGIN
			INSERT INTO event_write_count VALUES (NEW.id);
		END`); err != nil {
		t.Fatal(err)
	}
	e.OnEvent(linkUp)
	var updates int
	if err := st.DB.QueryRow(`SELECT count(*) FROM event_write_count`).Scan(&updates); err != nil || updates != 0 {
		t.Fatalf("状態変化のないラッチのDB更新: %d (%v)", updates, err)
	}
	for _, legacy := range []string{"null", "[]", `"legacy"`, "{"} {
		ev := alertEvent("LINK_DOWN", 2)
		e.OnEvent(ev)
		var id int64
		for _, a := range e.Alerts() {
			if a.Active && a.IfIndex == 2 {
				id = a.EventID
			}
		}
		if id == 0 {
			t.Fatal("検証対象のアラートが未発生")
		}
		if _, err := st.DB.Exec(`UPDATE events SET detail_json=? WHERE id=?`, legacy, id); err != nil {
			t.Fatal(err)
		}
		if err := e.SetPhase("setup"); err != nil {
			t.Fatal(err)
		}
		recovered := linkUp
		recovered.IfIndex = 2
		e.OnEvent(recovered)
		if err := e.SetPhase("show"); err != nil {
			t.Fatal(err)
		}
		var detail string
		if err := st.DB.QueryRow(`SELECT detail_json FROM events WHERE id=?`, id).Scan(&detail); err != nil || detail != `{"_suppressed":true}` {
			t.Fatalf("旧形式 %q の抑止保存: %s (%v)", legacy, detail, err)
		}
	}
	if alerts := New(st, e.ProfileID(), nil).Alerts(); len(alerts) != 1 || alerts[0].IfIndex != 1 {
		t.Fatalf("抑止中に解除したイベントが再起動で復活: %+v", alerts)
	}
}

func TestExcludedPortLinkEventsAreHistoryOnly(t *testing.T) {
	e, st := newTestEngine(t)
	e.OnEvent(alertEvent("LINK_DOWN", 1))
	e.OnEvent(alertEvent("LINK_FLAP", 1))
	e.OnEvent(alertEvent("LINK_DOWN", 2))
	if err := st.SavePortSetting(1, 1, store.PortSetting{Excluded: true}); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	for _, code := range []string{"LINK_DOWN", "LINK_FLAP"} {
		e.OnEvent(alertEvent(code, 1))
	}
	for _, current := range []*Engine{e, New(st, e.ProfileID(), nil)} {
		a := current.Alerts()
		if len(a) != 1 || a[0].IfIndex != 2 || current.Overview(false).Alerts[model.SevWarning] != 1 {
			t.Fatalf("監視除外ポートのラッチ: %+v", a)
		}
	}
	list, err := st.Events(store.EventFilter{})
	if err != nil || len(list) != 5 {
		t.Fatalf("履歴: %+v (%v)", list, err)
	}
	for _, ev := range list {
		if ev.IfIndex == 1 && ev.Detail["_suppressed"] != true {
			t.Fatalf("除外ポートの履歴が通知対象: %+v", ev)
		}
	}
}

func TestMissingDeviceUsesAccessPortAndFreshFDBOnOtherSwitch(t *testing.T) {
	for _, tc := range []struct {
		name, role string
		offset     time.Duration
		failed     bool
		wantAlert  bool
	}{
		{name: "fresh access", offset: time.Second},
		{name: "same time", wantAlert: true},
		{name: "stale access", offset: -time.Second, wantAlert: true},
		{name: "fresh trunk", role: "trunk", offset: time.Second, wantAlert: true},
		{name: "fresh failed", offset: time.Second, failed: true, wantAlert: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, db := newTestEngine(t)
			const mac = "00:11:22:33:44:55"
			registerDevice(t, e, db, mac, "")
			other := model.SwitchConfig{ProfileID: e.ProfileID(), Name: "Other", Enabled: true}
			if err := db.SaveSwitch(&other); err != nil {
				t.Fatal(err)
			}
			e.Reload()
			start := time.Now().UTC().Add(-time.Minute)
			valid, invalid := true, false
			state := func(id int64, ix int, role string, at time.Time, hasMAC bool) *model.SwitchState {
				p := &model.Port{IfIndex: ix, Name: "1/0/1", Oper: "up", AVRole: role}
				s := &model.SwitchState{ID: id, Reachable: true, Ports: []*model.Port{p}, LastPoll: at, FDBLastOK: at, FDBValid: &valid}
				if hasMAC {
					p.Devices = []model.Device{{MAC: mac, Source: "fdb"}}
					s.FDBMACs = []string{mac}
					s.FDBPorts = map[int][]string{ix: {mac}}
				}
				return s
			}
			e.OnState(state(1, 1, "", start.Add(-5*time.Second), true), "slow")
			if _, err := db.DB.Exec(`UPDATE device_bindings SET first_seen_ts=first_seen_ts-1000, last_seen_ts=last_seen_ts-1000 WHERE switch_id=1`); err != nil {
				t.Fatal(err)
			}
			e.OnState(state(other.ID, 2, "uplink", start.Add(-2*time.Second), true), "slow")
			var requests []int64
			e.SetFDBRequester(func(id int64, _ time.Time) { requests = append(requests, id) })
			e.OnEvent(alertEvent("LINK_DOWN", 1))
			e.OnState(state(1, 1, "", start, false), "slow")
			if len(requests) != 2 || requests[0] != 1 || requests[1] != 1 {
				t.Fatalf("リンク断と初回不在がアクセスポートの取得を要求しない: %+v", requests)
			}
			newObservation := state(other.ID, 2, tc.role, start.Add(tc.offset), true)
			if tc.failed {
				newObservation.FDBValid = &invalid
			}
			// FDB単独取得は統合済み機器一覧を更新しない。
			newObservation.Ports[0].Devices = nil
			e.OnState(newObservation, "fdb")
			e.OnState(state(1, 1, "", start.Add(10*time.Second), false), "fdb")
			if got := hasAlert(e, "DEVICE_MISSING"); got != tc.wantAlert {
				t.Fatalf("2回目不在の結果 = %v, want %v: %+v", got, tc.wantAlert, e.Alerts())
			}
			if !tc.wantAlert {
				// 移動先の次回Slowより先に旧スイッチで不在が続いても、旧所在には戻らない。
				e.OnState(state(1, 1, "", start.Add(20*time.Second), false), "slow")
				e.OnState(state(1, 1, "", start.Add(30*time.Second), false), "fdb")
				if got := codes(t, db)["DEVICE_MISSING"]; got != 0 {
					t.Fatalf("別スイッチで在席確認済みの消失履歴: %d", got)
				}
				ev := alertEvent("LINK_DOWN", 2)
				ev.SwitchID = other.ID
				e.OnEvent(ev)
				if len(requests) != 3 || requests[2] != other.ID {
					t.Fatalf("移動先のポートで追加取得を要求しない: %+v", requests)
				}
				return
			}
			if err := e.SetPhase("setup"); err != nil {
				t.Fatal(err)
			}
			e.OnState(state(other.ID, 2, "", start.Add(12*time.Second), true), "fdb")
			for _, a := range e.Alerts() {
				if a.Code == "DEVICE_MISSING" && a.SwitchID == 1 && !a.Active && !a.Acked && a.ClearedAt != nil {
					return
				}
			}
			t.Fatalf("別スイッチで復旧した未確認ラッチが残らない: %+v", e.Alerts())
		})
	}
}
