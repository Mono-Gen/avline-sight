package engine

import (
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func TestLatestIPIndependentOfLocationAcrossTiers(t *testing.T) {
	for _, tier := range []string{"fast", "medium", "slow"} {
		t.Run(tier, func(t *testing.T) {
			e, db := newTestEngine(t)
			const mac = "00:11:22:33:44:55"
			registerDevice(t, e, db, mac, "192.0.2.1")
			now := time.Now().Add(-time.Minute)
			db.UpsertBinding(1, 1, model.Device{MAC: mac, IP: "192.0.2.1"}, now)
			db.UpsertBinding(1, 2, model.Device{MAC: mac, IP: "192.0.2.2"}, now.Add(time.Second))
			s := &model.SwitchState{ID: 1, Reachable: true, Ports: []*model.Port{{IfIndex: 1}, {IfIndex: 2, AVRole: "uplink"}}}
			e.OnState(s, tier)
			if !hasAlert(e, "IP_MISMATCH") {
				t.Fatal("latest IP mismatch missing")
			}
			rows, err := e.Inventory(InventoryOptions{})
			if err != nil || len(rows) != 1 || rows[0].IP != "192.0.2.2" {
				t.Fatalf("inventory: %+v %v", rows, err)
			}
		})
	}
}

func TestLocationPreferenceAndUplinkNeverMissing(t *testing.T) {
	e, db := newTestEngine(t)
	const mac = "00:11:22:33:44:55"
	registerDevice(t, e, db, mac, "")
	other := model.SwitchConfig{ProfileID: e.ProfileID(), Name: "disabled", Enabled: false}
	if err := db.SaveSwitch(&other); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Minute)
	db.UpsertBinding(other.ID, 3, model.Device{MAC: mac}, now)
	db.UpsertBinding(1, 2, model.Device{MAC: mac}, now.Add(time.Second))
	s := &model.SwitchState{ID: 1, Reachable: true, Ports: []*model.Port{{IfIndex: 2, AVRole: "trunk"}}}
	e.OnState(s, "fast")
	ports := accessPorts(e.States())
	last, err := db.LastSeenByMACPreferringPorts(e.ProfileID(), time.Now(), ports.rank)
	if err != nil || last[mac].SwitchID != other.ID {
		t.Fatalf("unknown must beat uplink: %+v %v", last, err)
	}
	db.UpsertBinding(1, 1, model.Device{MAC: mac}, now.Add(-time.Second))
	s.Ports = append(s.Ports, &model.Port{IfIndex: 1})
	ports = accessPorts([]*model.SwitchState{s})
	last, err = db.LastSeenByMACPreferringPorts(e.ProfileID(), time.Now(), ports.rank)
	if err != nil || last[mac].IfIndex != 1 || last[mac].SwitchID != 1 {
		t.Fatalf("access must beat unknown: %+v %v", last, err)
	}

	e2, db2 := newTestEngine(t)
	registerDevice(t, e2, db2, mac, "")
	db2.UpsertBinding(1, 2, model.Device{MAC: mac}, now)
	for i := 0; i < 2; i++ {
		e2.OnState(&model.SwitchState{ID: 1, Reachable: true, Ports: []*model.Port{{IfIndex: 2, AVRole: "trunk"}}}, "slow")
	}
	if hasAlert(e2, "DEVICE_MISSING") || codes(t, db2)["DEVICE_MISSING"] != 0 {
		t.Fatal("uplink created missing alert")
	}
}

func TestExcludedPortTransientEventsAreHistoryOnly(t *testing.T) {
	e, db := newTestEngine(t)
	if err := db.SavePortSetting(1, 1, store.PortSetting{Excluded: true}); err != nil {
		t.Fatal(err)
	}
	e.Reload()
	for _, code := range []string{"RELINK_SLOWER", "SPEED_CHANGE", "POE_STATE", "POE_FAULT"} {
		e.OnEvent(alertEvent(code, 1))
	}
	if len(e.Alerts()) != 0 || len(New(db, e.ProfileID(), nil).Alerts()) != 0 {
		t.Fatal("excluded events latched")
	}
	events, err := db.Events(store.EventFilter{})
	if err != nil || len(events) != 4 {
		t.Fatalf("history: %+v %v", events, err)
	}
	for _, ev := range events {
		if ev.Detail["_suppressed"] != true {
			t.Fatalf("not suppressed: %+v", ev)
		}
	}
}
