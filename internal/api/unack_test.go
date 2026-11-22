package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func TestWallAcknowledgementUndo(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		t.Run(fmt.Sprint("recovered=", recovered), func(t *testing.T) {
			s, h := newDevicesServer(t)
			s.Engine.OnEvent(model.Event{SwitchID: 1, IfIndex: 1, Category: "CONNECT", Severity: model.SevWarning, Code: "LINK_DOWN", Title: "link down"})
			id := s.Engine.Alerts()[0].EventID
			if recovered {
				s.Engine.OnState(&model.SwitchState{ID: 1, Reachable: true, Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Oper: "up"}}}, "fast")
			}
			for _, action := range []string{"ack", "unack", "unack"} {
				w := do(h, "POST", fmt.Sprintf("/api/events/%d/%s", id, action), "")
				if w.Code != 200 {
					t.Fatalf("%s: %d %s", action, w.Code, w.Body)
				}
			}
			a := s.Engine.Alerts()
			if len(a) != 1 || a[0].Acked || a[0].Active == recovered || recovered && a[0].ClearedAt == nil {
				t.Fatalf("undo did not restore the alert: %+v", a)
			}
			var acked bool
			var ackedAt any
			if err := s.Store.DB.QueryRow("SELECT acked,acked_at FROM events WHERE id=?", id).Scan(&acked, &ackedAt); err != nil || acked || ackedAt != nil {
				t.Fatalf("persisted acknowledgement: %v %v %v", acked, ackedAt, err)
			}
			if w := do(h, "POST", "/api/mode/lock", `{"locked":true}`); w.Code != 200 {
				t.Fatal(w.Body)
			}
			for _, action := range []string{"ack", "unack"} {
				if w := do(h, "POST", fmt.Sprintf("/api/events/%d/%s", id, action), ""); w.Code != 423 {
					t.Fatalf("locked %s: %d", action, w.Code)
				}
			}
		})
	}
}

func TestUndoRejectsUnavailableEventsAndCrossOrigin(t *testing.T) {
	s, h := newDevicesServer(t)
	for _, id := range []string{"0", "-1", "bad"} {
		if w := do(h, "POST", "/api/events/"+id+"/unack", ""); w.Code != 400 {
			t.Fatalf("invalid ID %s: %d", id, w.Code)
		}
	}
	if w := do(h, "POST", "/api/events/999999/unack", ""); w.Code != 404 {
		t.Fatal(w.Code)
	}
	s.Engine.OnEvent(model.Event{SwitchID: 1, Category: "CONNECT", Severity: model.SevWarning, Code: "LINK_DOWN"})
	id := s.Engine.Alerts()[0].EventID
	path := fmt.Sprintf("/api/events/%d/unack", id)
	r := httptest.NewRequest("POST", path, nil)
	r.Header.Set("Origin", "https://other.invalid")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatal(w.Code)
	}
	if err := s.Engine.Ack(id, false); err != nil {
		t.Fatal(err)
	}
	sws, err := s.Store.Switches(s.Engine.ProfileID())
	if err != nil || len(sws) != 1 {
		t.Fatalf("switches: %v %v", sws, err)
	}
	sw := sws[0]
	sw.Enabled = false
	if err := s.Store.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	if w := do(h, "POST", path, ""); w.Code != 404 {
		t.Fatalf("disabled switch: %d", w.Code)
	}
	other := model.Event{SwitchID: 0, Severity: model.SevWarning, Category: "ANOMALY", Detail: map[string]any{"_profile_id": s.Engine.ProfileID() + 1}}
	if _, err := s.Store.InsertEvent(&other); err != nil {
		t.Fatal(err)
	}
	if w := do(h, "POST", fmt.Sprintf("/api/events/%d/unack", other.ID), ""); w.Code != 404 {
		t.Fatalf("other profile: %d", w.Code)
	}
	if events, err := s.Store.Events(store.EventFilter{ID: id}); err != nil || len(events) != 1 || !events[0].Acked {
		t.Fatalf("rejected undo changed history: %v %v", events, err)
	}
}
