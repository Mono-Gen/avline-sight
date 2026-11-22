package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
)

func TestRenameAndOrderGroups(t *testing.T) {
	s, h := newDevicesServer(t)
	pid := s.Engine.ProfileID()
	for _, name := range []string{"Z", "Audio / Main", "Video", "order"} {
		for i := 0; i < 2; i++ {
			sw := &model.SwitchConfig{ProfileID: pid, Name: name, IP: "192.0.2.42", GroupTag: name, Enabled: true}
			if err := s.Store.SaveSwitch(sw); err != nil {
				t.Fatal(err)
			}
		}
	}
	check := func(method, path, body string, code int) []engine.Group {
		t.Helper()
		w := do(h, method, path, body)
		if w.Code != code {
			t.Fatalf("%s %s: got %d, want %d (%s)", method, path, w.Code, code, w.Body)
		}
		var groups []engine.Group
		if code == 200 && path != "/api/mode/lock" && json.Unmarshal(w.Body.Bytes(), &groups) != nil {
			t.Fatalf("グループ応答: %s", w.Body)
		}
		return groups
	}
	names := func(groups []engine.Group) []string {
		out := make([]string, 0, len(groups))
		for _, group := range groups {
			out = append(out, group.Name)
		}
		return out
	}
	groupPath := "/api/groups/Audio%20%2F%20Main"
	check("PUT", groupPath, `{"new_name":" "}`, 400)
	check("PUT", "/api/groups/Missing", `{"new_name":"Audio"}`, 404)
	check("PUT", "/api/groups/order", `{"names":["Z","Z"]}`, 400)
	check("PUT", "/api/groups/order", `{"names":["Z","Audio / Main"]}`, 200)
	if got := names(check("GET", "/api/groups", "", 200)); !reflect.DeepEqual(got, []string{"Z", "Audio / Main", "Video", "order", ""}) {
		t.Fatalf("表示順: %v", got)
	}
	groups := check("PUT", groupPath, `{"new_name":" Video "}`, 200)
	if got := names(groups); !reflect.DeepEqual(got, []string{"Z", "Video", "order", ""}) || len(groups[1].SwitchIDs) != 4 {
		t.Fatalf("統合後: %+v", groups)
	}
	check("PUT", "/api/groups/order", `{"new_name":"Lighting"}`, 200)
	check("POST", "/api/mode/lock", `{"locked":true}`, 200)
	check("PUT", "/api/groups/order", `{"names":[]}`, 423)
	check("PUT", "/api/groups/Video", `{"new_name":"Z"}`, 423)
}

func TestGroupChangesRejectStaleProfile(t *testing.T) {
	s, h := newDevicesServer(t)
	original := s.Engine.ProfileID()
	other, err := s.Store.CreateProfile("別の現場")
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range []int64{original, other} {
		sw := &model.SwitchConfig{ProfileID: profile, Name: "SW", GroupTag: "Audio", IP: "192.0.2.43", Enabled: true}
		if err := s.Store.SaveSwitch(sw); err != nil {
			t.Fatal(err)
		}
	}
	w := do(h, "PUT", fmt.Sprintf("/api/profiles/%d/activate", other), "")
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	for _, request := range []struct{ method, path, body string }{
		{"PUT", "/api/groups/Audio", `{"new_name":"Changed"}`},
		{"PUT", "/api/groups/order", `{"names":["Audio"]}`},
		{"POST", "/api/counters/reset", `{"group":"*"}`},
	} {
		req := httptest.NewRequest(request.method, request.path, strings.NewReader(request.body))
		req.Header.Set("X-AVLine-Profile", strconv.FormatInt(original, 10))
		req.Header.Set("Accept-Language", "en")
		result := httptest.NewRecorder()
		h.ServeHTTP(result, req)
		if result.Code != 409 || !strings.Contains(result.Body.String(), "The active profile has changed") {
			t.Fatalf("%s %s: %d %s", request.method, request.path, result.Code, result.Body)
		}
	}
	for _, profile := range []int64{original, other} {
		switches, err := s.Store.Switches(profile)
		if err != nil {
			t.Fatal(err)
		}
		for _, sw := range switches {
			if sw.GroupTag == "Changed" {
				t.Fatalf("別現場の機器を変更: %+v", sw)
			}
		}
	}
}

func TestLegacyReservedGroupCanBeRenamedWithoutTouchingAll(t *testing.T) {
	s, h := newDevicesServer(t)
	sw := &model.SwitchConfig{ProfileID: s.Engine.ProfileID(), Name: "Legacy", GroupTag: "*", IP: "192.0.2.44", Enabled: true}
	if err := s.Store.SaveSwitch(sw); err != nil {
		t.Fatal(err)
	}
	w := do(h, "PUT", "/api/groups/*", `{"new_name":"Audio"}`)
	var groups []engine.Group
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &groups) != nil ||
		len(groups) != 2 || groups[0].Name != "Audio" || len(groups[0].SwitchIDs) != 1 {
		t.Fatal(w.Body)
	}
}

func TestRenamedGroupInventoryAndCSVSurvivePolling(t *testing.T) {
	s, h := newDevicesServer(t)
	sws, err := s.Store.Switches(s.Engine.ProfileID())
	if err != nil {
		t.Fatal(err)
	}
	sw := sws[0]
	sw.GroupTag = "Audio"
	if err := s.Store.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	s.Engine.Reload()
	if w := do(h, "PUT", "/api/groups/Audio", `{"new_name":"Video"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	s.Engine.OnState(&model.SwitchState{ID: sw.ID, Name: sw.Name, GroupTag: "Audio", Reachable: true,
		Ports: []*model.Port{{IfIndex: 1, Name: "1/0/1", Devices: []model.Device{{MAC: "00:11:22:33:44:55", IP: "192.0.2.10"}}}}}, "slow")
	w := do(h, "GET", "/api/devices?group=Video", "")
	var rows []engine.InventoryRow
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rows) != nil || len(rows) != 1 || rows[0].GroupTag != "Video" || rows[0].IP != "192.0.2.10" {
		t.Fatalf("絞込と一括Pingに使う宛先一覧: %s", w.Body)
	}
	w = do(h, "GET", "/api/devices/export.csv?group=Video&lang=en", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "00:11:22:33:44:55") || !strings.Contains(w.Body.String(), "192.0.2.10") {
		t.Fatalf("改名後のCSVから機器が消えた: %s", w.Body)
	}
}
