package engine

import (
	"context"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/ping"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func TestTransientFaultsLatchUntilAck(t *testing.T) {
	for _, code := range []string{"SWITCH_REBOOT", "POE_FAULT", "RELINK_SLOWER", "SPEED_CHANGE"} {
		t.Run(code, func(t *testing.T) {
			e, st := newTestEngine(t)
			ev := alertEvent(code, 1)
			ev.Severity = model.SevCritical
			e.OnEvent(ev)
			e.OnState(&model.SwitchState{ID: 1, Reachable: true, Ports: []*model.Port{{IfIndex: 1, Oper: "up"}}}, "fdb")
			a := e.Alerts()
			if len(a) != 1 || a[0].Active || a[0].Acked || a[0].State != "latched" || e.Overview(false).Alerts[model.SevCritical] != 1 {
				t.Fatalf("一過性障害は正常な取得後も未確認で残る: %+v", a)
			}
			e.OnEvent(ev)
			e = New(st, e.ProfileID(), nil)
			if len(e.Alerts()) != 2 {
				t.Fatalf("別々の発生を再起動後も保持: %+v", e.Alerts())
			}
			if err := e.Ack(a[0].EventID, false); err != nil || len(e.Alerts()) != 1 {
				t.Fatalf("確認で当該ラッチのみ削除: %v %+v", err, e.Alerts())
			}
		})
	}
}

func TestTransientFaultSuppressionAndRawTrapSoundFlag(t *testing.T) {
	for _, mode := range []string{"setup", "teardown", "work"} {
		t.Run(mode, func(t *testing.T) {
			e, st := newTestEngine(t)
			if mode == "work" {
				if err := e.SetWork("*", 15); err != nil {
					t.Fatal(err)
				}
			} else if err := e.SetPhase(mode); err != nil {
				t.Fatal(err)
			}
			for _, category := range []string{"ANOMALY", "TRAP"} {
				ev := alertEvent("POE_FAULT", 1)
				ev.Category, ev.Severity = category, model.SevCritical
				e.OnEvent(ev)
			}
			if len(e.Alerts()) != 0 {
				t.Fatalf("抑止中にラッチ: %+v", e.Alerts())
			}
			list, err := st.Events(store.EventFilter{Severity: model.SevCritical})
			if err != nil || len(list) != 2 {
				t.Fatalf("履歴: %+v %v", list, err)
			}
			for _, ev := range list {
				if ev.Detail["_suppressed"] != true {
					t.Fatalf("通知音向けの抑止フラグなし: %+v", ev)
				}
			}
			if err := e.EndWork("*"); err != nil {
				t.Fatal(err)
			}
			if err := e.SetPhase("show"); err != nil {
				t.Fatal(err)
			}
			if len(New(st, e.ProfileID(), nil).Alerts()) != 0 {
				t.Fatal("抑止解除後の再起動で復活")
			}
		})
	}
}

func TestLinkFlapAfterSuppressionEndsLatches(t *testing.T) {
	for _, mode := range []string{"work", "phase"} {
		t.Run(mode, func(t *testing.T) {
			e, st := newTestEngine(t)
			if mode == "work" {
				if err := e.SetWork("*", 15); err != nil {
					t.Fatal(err)
				}
			} else if err := e.SetPhase("teardown"); err != nil {
				t.Fatal(err)
			}
			ev := alertEvent("LINK_FLAP", 1)
			ev.TS = time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
			e.OnEvent(ev)
			if mode == "work" {
				if err := e.EndWork("*"); err != nil {
					t.Fatal(err)
				}
			} else if err := e.SetPhase("show"); err != nil {
				t.Fatal(err)
			}
			ev.TS = ev.TS.Add(time.Second)
			e.OnEvent(ev)
			if a := e.Alerts(); len(a) != 1 || a[0].Suppressed || a[0].Active || !a[0].TS.Equal(ev.TS) {
				t.Fatalf("60秒以内でも解除後の発生は通知対象: %+v", a)
			}
			if codes(t, st)["LINK_FLAP"] != 2 {
				t.Fatal("抑止中と解除後の履歴は分ける")
			}
			if len(New(st, e.ProfileID(), nil).Alerts()) != 1 {
				t.Fatal("再起動で通知対象を失った")
			}
		})
	}
}

func TestManualPingClearsFailureInShow(t *testing.T) {
	for _, ackFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "unacked", true: "acked"}[ackFirst], func(t *testing.T) {
			e, st := newTestEngine(t)
			const mac = "00:11:22:33:44:55"
			registerDevice(t, e, st, mac, "192.0.2.10")
			e.SetPinger(&fakePinger{status: ping.StatusTimeout})
			if err := e.SetPhase("rehearsal"); err != nil {
				t.Fatal(err)
			}
			e.periodicPing(context.Background())
			e.periodicPing(context.Background())
			if !hasAlert(e, "PING_FAIL") {
				t.Fatal("2回失敗で発生しない")
			}
			if ackFirst {
				if err := e.Ack(e.Alerts()[0].EventID, false); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.SetPhase("show"); err != nil {
				t.Fatal(err)
			}
			e.RecordManualPing(e.ProfileID(), map[string]PingMark{"192.0.2.10": {Status: "timeout", At: time.Now()}})
			if !hasAlert(e, "PING_FAIL") {
				t.Fatal("手動失敗で解除された")
			}
			e.RecordManualPing(e.ProfileID()+1, map[string]PingMark{"192.0.2.10": {Status: "ok", At: time.Now()}})
			if !hasAlert(e, "PING_FAIL") {
				t.Fatal("別プロファイルの成功で解除された")
			}
			e.RecordManualPing(e.ProfileID(), map[string]PingMark{"192.0.2.10": {Status: "ok", At: time.Now()}})
			a := e.Alerts()
			if hasAlert(e, "PING_FAIL") || (ackFirst && len(a) != 0) || (!ackFirst && (len(a) != 1 || a[0].State != "latched" || a[0].ClearedAt == nil)) {
				t.Fatalf("手動成功は条件を解除し未確認のみ保持: %+v", a)
			}
			if e.pingFails[mac] != 0 {
				t.Fatal("失敗回数がリセットされない")
			}
		})
	}
}

func TestRegisteredGroupOverridesStalePollState(t *testing.T) {
	e, st := newTestEngine(t)
	sws, err := st.Switches(e.ProfileID())
	if err != nil {
		t.Fatal(err)
	}
	sw := sws[0]
	sw.GroupTag = "Audio"
	if err := st.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	sw.GroupTag = "Video"
	if err := st.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	e.OnState(&model.SwitchState{ID: sw.ID, Name: "SW", GroupTag: "Audio", Reachable: true,
		Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Devices: []model.Device{{MAC: "00:11:22:33:44:55", IP: "192.0.2.10"}}}}}, "slow")
	rows, err := e.Inventory(InventoryOptions{Group: "Video"})
	if err != nil || len(rows) != 1 || rows[0].GroupTag != "Video" || e.State(sw.ID).GroupTag != "Video" {
		t.Fatalf("古いワーカー出力より登録済みグループを優先: %+v %v", rows, err)
	}
}

func TestClearedAtSurvivesRestartAndReactivation(t *testing.T) {
	e, st := newTestEngine(t)
	ev := alertEvent("TEST", 1)
	ev.TS = time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	e.raise(map[string]bool{}, "test", ev)
	e.clearPrefix("test", map[string]bool{})
	cleared := e.Alerts()[0].ClearedAt
	if cleared == nil || !cleared.After(ev.TS) {
		t.Fatal("復旧時刻が発生時刻より後でない")
	}
	e = New(st, e.ProfileID(), nil)
	a := e.Alerts()[0]
	if a.ClearedAt == nil || a.ClearedAt.UnixMilli() != cleared.UnixMilli() {
		t.Fatalf("復旧時刻が変化: %+v want %v", a, cleared)
	}
	e.raise(map[string]bool{}, "test", ev)
	if e.Alerts()[0].ClearedAt != nil {
		t.Fatal("再発生で復旧時刻を消していない")
	}
	e = New(st, e.ProfileID(), nil)
	if e.Alerts()[0].ClearedAt != nil {
		t.Fatal("再評価前の復旧時刻を捏造した")
	}
}
