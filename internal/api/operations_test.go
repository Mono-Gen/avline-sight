package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/trap"
)

func TestOperationEndpointsLocksAndSync(t *testing.T) {
	s, h := newDevicesServer(t)
	s.Hub = NewHub()
	c := &client{send: make(chan []byte, 128)}
	s.Hub.clients[c] = true
	s.Engine = engine.New(s.Store, s.Engine.ProfileID(), s.Hub)
	s.Trap = &trap.Receiver{}
	if w := do(h, "GET", "/api/phase", ""); w.Code != 200 || w.Body.String() != "{\"phase\":\"show\"}\n" {
		t.Fatalf("既定: %d %s", w.Code, w.Body)
	}
	if w := do(h, "PUT", "/api/phase", `{"phase":"wrong"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := do(h, "PUT", "/api/phase", `{"phase":"setup"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := do(h, "GET", "/api/status", ""); w.Code != 200 {
		t.Fatal(w.Body)
	} else {
		var status struct{ Phase string }
		if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil || status.Phase != "setup" {
			t.Fatalf("status: %s %v", w.Body, err)
		}
	}
	if w := do(h, "POST", "/api/counters/reset", `{"group":"*"}`); w.Code != 200 {
		t.Fatal(w.Body)
	} else {
		var body struct {
			Results []struct {
				SwitchID int64 `json:"switch_id"`
				OK       bool
				Error    string
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Results) != 1 || body.Results[0].OK || body.Results[0].Error == "" {
			t.Fatalf("非監視結果: %s %v", w.Body, err)
		}
	}
	if w := do(h, "POST", "/api/mode/lock", `{"locked":true}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	for _, tc := range []struct{ method, path, body string }{
		{"PUT", "/api/phase", `{"phase":"show"}`},
		{"POST", "/api/counters/reset", `{"group":"*"}`},
	} {
		if w := do(h, tc.method, tc.path, tc.body); w.Code != 423 {
			t.Fatalf("ロック %s: %d", tc.path, w.Code)
		}
	}
	s.Engine.OnEvent(model.Event{SwitchID: 1, IfIndex: 1, Category: "CONNECT", Severity: model.SevWarning, Code: "LINK_DOWN", Title: "リンク断"})
	a := s.Engine.Alerts()
	if len(a) != 1 {
		t.Fatal(a)
	}
	if w := do(h, "POST", "/api/events/ack-all", ""); w.Code != 423 {
		t.Fatalf("ロック中の一括確認: %d", w.Code)
	}
	if w := do(h, "POST", "/api/mode/lock", `{"locked":false}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := do(h, "POST", "/api/events/ack-all", ""); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if !s.Engine.Alerts()[0].Acked {
		t.Fatal("メモリ未反映")
	}
	if w := do(h, "GET", "/api/alerts", ""); w.Code != 200 {
		t.Fatal(w.Body)
	}
	types := map[string]bool{}
	for len(c.send) > 0 {
		var msg struct{ Type string }
		if err := json.Unmarshal(<-c.send, &msg); err != nil {
			t.Fatal(err)
		}
		types[msg.Type] = true
	}
	for _, kind := range []string{"phase_changed", "lock_changed", "alerts_changed"} {
		if !types[kind] {
			t.Fatalf("WS通知なし %s", kind)
		}
	}
}

func TestPlannedIPAPIAndEscapedGroup(t *testing.T) {
	s, h := newDevicesServer(t)
	if w := do(h, "POST", "/api/device-aliases", `{"match_type":"mac","match_key":"00:11:22:33:44:55","label":"卓","planned_ip":"bad"}`); w.Code != 400 {
		t.Fatal(w.Body)
	}
	if w := do(h, "POST", "/api/device-aliases", `{"match_type":"mac","match_key":"00:11:22:33:44:55","label":"卓","planned_ip":"192.0.2.12"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if !s.pingAllowed()["192.0.2.12"] {
		t.Fatal("予定IPがPing対象外")
	}
	if w := do(h, "GET", "/api/devices?group=*", ""); w.Code != 200 {
		t.Fatal(w.Body)
	} else {
		var rows []engine.InventoryRow
		if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil || len(rows) != 1 || rows[0].PlannedIP != "192.0.2.12" {
			t.Fatalf("devices: %s %v", w.Body, err)
		}
	}
	if w := do(h, "GET", "/api/devices/export.csv?group=*", ""); w.Code != 200 {
		t.Fatal(w.Body)
	}
	sws, err := s.Store.Switches(s.Engine.ProfileID())
	if err != nil {
		t.Fatal(err)
	}
	sw := sws[0]
	sw.GroupTag = "音響 / A"
	if err := s.Store.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	s.Engine.Reload()

}

func TestLastTrapTimestampAPI(t *testing.T) {
	s, h := newDevicesServer(t)
	assert := func(path string, before bool) {
		t.Helper()
		w := do(h, "GET", path, "")
		if w.Code != 200 {
			t.Fatal(w.Body)
		}
		var list []struct {
			LastTrap *time.Time `json:"last_trap_ts"`
		}
		if path == "/api/overview" {
			var overview struct{ Switches json.RawMessage }
			if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(overview.Switches, &list); err != nil {
				t.Fatal(err)
			}
		} else if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(err)
		}
		if len(list) != 1 || (list[0].LastTrap == nil) != before {
			t.Fatalf("%s: %s", path, w.Body)
		}
	}
	assert("/api/switches", true)
	assert("/api/overview", true)
	s.Engine.OnEvent(model.Event{SwitchID: 1, Category: "TRAP", Severity: model.SevInfo, Code: "authenticationFailure"})
	assert("/api/switches", false)
	assert("/api/overview", false)
}
