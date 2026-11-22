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
	check("POST", "/api/work", `{"group":"Audio / Main","minutes":15}`, 200)
	check("PUT", groupPath, `{"new_name":"Video"}`, 409)
	check("DELETE", "/api/work/Audio%20%2F%20Main", "", 200)
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
		{"POST", "/api/work", `{"group":"Audio","minutes":15}`},
		{"DELETE", "/api/work/Audio", ""},
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
	if w := do(h, "POST", "/api/work", `{"group":"*","minutes":15}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	w := do(h, "PUT", "/api/groups/*", `{"new_name":"Audio"}`)
	var groups []engine.Group
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &groups) != nil ||
		len(groups) != 2 || groups[0].Name != "Audio" || len(groups[0].SwitchIDs) != 1 ||
		groups[0].WorkUntil == nil || groups[1].WorkUntil == nil {
		t.Fatal(w.Body)
	}
}
