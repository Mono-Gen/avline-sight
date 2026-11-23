package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

type profilePreviewResponse struct {
	ProfileID int64        `json:"profile_id"`
	Revision  string       `json:"revision"`
	Switches  int          `json:"switch_count"`
	Groups    int          `json:"group_count"`
	Changes   []configDiff `json:"changes"`
}

const sampleProfile = `{"version":1,"name":"Stage","group_order":["Video","Audio"],
	"switches":[{"id":42,"name":"Core","ip":"192.0.2.7","group_tag":"Audio","snmp_port":161,"enabled":true},
	{"id":53,"name":"Core","ip":"192.0.2.8","group_tag":"Video","snmp_port":1161,"enabled":false}],
	"settings":{"setup_ping_interval_sec":30,"rehearsal_ping_interval_sec":3600,"expert_display_default":true,"thresholds":{"cpu_pct":88}},
	"trap_addr":"127.0.0.1:1162",
	"ports":{"42":{"10":{"excluded":true}},"53":{"10":{"expected_speed_mbps":1000}}},
	"watchlist":[{"switch_id":53,"if_index":10,"label":"Link"}],
	"topology_layout":{"sw-42":{"X":50,"Y":75}},
	"aliases":[{"match_type":"port","match_key":"42:10","label":"Uplink"}]}`

func previewConfig(t *testing.T, h http.Handler, body string) profilePreviewResponse {
	t.Helper()
	w := do(h, "POST", "/api/profiles/import/preview", body)
	var preview profilePreviewResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &preview) != nil || preview.Revision == "" {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	return preview
}

func applyConfig(h http.Handler, body string, preview profilePreviewResponse) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/api/profiles/import", strings.NewReader(body))
	r.Header.Set("X-AVLine-Profile", fmt.Sprint(preview.ProfileID))
	r.Header.Set("X-AVLine-Config-Revision", preview.Revision)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestProfileConfigRoundTripAndCredentials(t *testing.T) {
	s, h := newDevicesServer(t)
	sampleProfile := strings.Replace(sampleProfile, `"42:10","label":"Uplink"}`, `"042:+010","label":"Uplink"},{"match_type":"mac","match_key":"aabb.ccdd.eeff","label":"Mixer"}`, 1)
	oldID := s.Engine.ProfileID()
	preview := previewConfig(t, h, sampleProfile)
	diff := map[string]configDiff{}
	for _, change := range preview.Changes {
		diff[change.Field] = change
	}
	if preview.ProfileID != oldID || preview.Switches != 2 || preview.Groups != 2 ||
		diff["settings.setup_ping_interval_sec"].Before != float64(60) ||
		diff["settings.setup_ping_interval_sec"].After != float64(30) ||
		diff["settings.expert_display_default"].After != true {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if profiles, _ := s.Store.Profiles(); len(profiles) != 1 {
		t.Fatal("preview wrote a profile")
	}
	if w := applyConfig(h, sampleProfile, preview); w.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	id := s.Engine.ProfileID()
	if id == oldID {
		t.Fatal("imported profile did not activate")
	}
	settings := s.Engine.Settings()
	if settings.SetupPingIntervalSec != 30 || settings.RehearsalPingIntervalSec != 3600 ||
		settings.Thresholds.CPUPct != 88 || settings.Thresholds.MemFreePct != 15 || !settings.ExpertDisplayDefault {
		t.Fatal(settings)
	}
	var listed []struct {
		ID               int64
		NeedsCredentials bool `json:"needs_credentials"`
		Enabled          bool
	}
	w := do(h, "GET", "/api/switches", "")
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil || len(listed) != 2 ||
		!listed[0].NeedsCredentials || !listed[1].NeedsCredentials || !listed[0].Enabled || listed[1].Enabled {
		t.Fatalf("switches: %s", w.Body)
	}
	w = do(h, "GET", "/api/status", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"credentials_pending":2`) {
		t.Fatalf("status: %s", w.Body)
	}
	w = do(h, "GET", "/api/groups", "")
	if w.Code != 200 || strings.Index(w.Body.String(), "Video") >= strings.Index(w.Body.String(), "Audio") {
		t.Fatalf("group order: %s", w.Body)
	}
	first, second := listed[0].ID, listed[1].ID
	ports1, _ := s.Store.PortSettings(first)
	ports2, _ := s.Store.PortSettings(second)
	if !ports1[10].Excluded || ports1[10].ExpectedSpeed != 0 || ports2[10].Excluded || ports2[10].ExpectedSpeed != 1000 {
		t.Fatalf("ports: %v %v", ports1, ports2)
	}
	watches := []store.ConfigWatch{}
	layout := map[string]store.ConfigPosition{}
	if err := s.Store.ProfileJSON(id, "watchlist_json", &watches); err != nil || len(watches) != 1 || watches[0].SwitchID != second {
		t.Fatalf("watchlist: %v %v", watches, err)
	}
	if err := s.Store.ProfileJSON(id, "topology_layout_json", &layout); err != nil ||
		layout[fmt.Sprintf("sw-%d", first)] != (store.ConfigPosition{X: 50, Y: 75}) {
		t.Fatalf("layout: %v %v", layout, err)
	}
	assertSavedAlias := func() {
		t.Helper()
		aliases, err := s.Store.Aliases(s.Engine.ProfileID())
		if err != nil || len(aliases) != 2 || aliases[0].MatchKey != "AA:BB:CC:DD:EE:FF" ||
			aliases[1].MatchKey != fmt.Sprintf("%d:10", first) {
			t.Fatalf("aliases: %v %v", aliases, err)
		}
	}
	assertSavedAlias()
	w = do(h, "PUT", fmt.Sprintf("/api/switches/%d", first), `{"name":"Core","ip":"192.0.2.7","group_tag":"Audio","snmp_port":161,"enabled":true,"community":"sample-snmp-secret","web_url":"http://user:sample-url-secret@192.0.2.7/manage?token=sample-query-secret"}`)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if err := s.Store.RecordTrap(first, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.InsertEvent(&model.Event{TS: time.Now(), Category: "SYSTEM", Severity: model.SevInfo, Title: "sample-history"}); err != nil {
		t.Fatal(err)
	}
	rawSettings := map[string]any{"setup_ping_interval_sec": 30, "rehearsal_ping_interval_sec": 3600,
		"thresholds": map[string]any{"cpu_pct": 88}, "expert_display_default": true, "theme": "sample-browser-theme", "snmpv3_key": "sample-v3-key"}
	if err := s.Store.SetProfileJSON(id, "settings_json", rawSettings); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/profiles/%d/export", id)
	w = do(h, "GET", path, "")
	exported := w.Body.String()
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "avline-sight.config.json") ||
		!strings.Contains(exported, `"version":1`) || !strings.Contains(exported, `"web_url":"http://192.0.2.7/manage"`) {
		t.Fatalf("export: %d %s", w.Code, exported)
	}
	for _, forbidden := range []string{"sample-snmp-secret", "sample-url-secret", "sample-query-secret", "sample-v3-key",
		"sample-history", "sample-browser-theme", "last_trap_ts", "community", "snmpv3_key"} {
		if strings.Contains(exported, forbidden) {
			t.Fatalf("export includes %q", forbidden)
		}
	}
	preview = previewConfig(t, h, exported)
	if len(preview.Changes) != 0 {
		t.Fatalf("round-trip preview added changes: %+v", preview.Changes)
	}
	if w := applyConfig(h, exported, preview); w.Code != 200 {
		t.Fatalf("round-trip apply: %s", w.Body)
	}
	w = do(h, "GET", fmt.Sprintf("/api/profiles/%d/export", s.Engine.ProfileID()), "")
	if w.Code != 200 || w.Body.String() != exported {
		t.Fatalf("round-trip export: %s", w.Body)
	}
}

func TestProfileConfigRejectsInvalidOrDuplicateAliasKeys(t *testing.T) {
	for _, tc := range []struct{ name, aliases, field string }{
		{"duplicate MAC", `{"match_type":"mac","match_key":"aa-bb-cc-dd-ee-ff","label":"A"},
			{"match_type":"mac","match_key":"aabb.ccdd.eeff","label":"B"}`, "aliases[1].match_key: duplicate alias"},
		{"duplicate port", `{"match_type":"port","match_key":"42:10","label":"A"},
			{"match_type":"port","match_key":"+042:010","label":"B"}`, "aliases[1].match_key: duplicate alias"},
		{"eight-byte MAC", `{"match_type":"mac","match_key":"00:11:22:33:44:55:66:77","label":"A"}`,
			"aliases[0].match_key: expected a 6-byte MAC address"},
		{"port index overflow", `{"match_type":"port","match_key":"42:99999999999999999999","label":"A"}`,
			"aliases[0].match_key: invalid switch or port reference"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Replace(sampleProfile, `{"match_type":"port","match_key":"42:10","label":"Uplink"}`, tc.aliases, 1)
			if _, err := engine.ReadProfileConfig(strings.NewReader(body)); err == nil || err.Error() != tc.field {
				t.Fatalf("error = %v, want %q", err, tc.field)
			}
		})
	}
}

func TestProfileConfigValidationAndLock(t *testing.T) {
	s, h := newDevicesServer(t)
	ctx := context.Background()
	pid := s.Engine.ProfileID()
	original, err := s.Store.ProfileConfig(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	preview := previewConfig(t, h, sampleProfile)
	for _, tc := range []struct{ old, bad, field string }{
		{`"version":1`, `"version":2`, "version"},
		{`"setup_ping_interval_sec":30`, `"setup_ping_interval_sec":29`, "setup_ping_interval_sec"},
		{`"rehearsal_ping_interval_sec":3600`, `"rehearsal_ping_interval_sec":3601`, "rehearsal_ping_interval_sec"},
		{`"setup_ping_interval_sec":30`, `"setup_ping_interval_sec":null`, "setup_ping_interval_sec"},
		{`"cpu_pct":88`, `"cpu_pct":101`, "thresholds.cpu_pct"},
		{`"ip":"192.0.2.8"`, `"ip":"host/invalid"`, "switches[1].ip"},
		{`"snmp_port":1161`, `"snmp_port":65536`, "switches[1].snmp_port"},
		{`"enabled":false`, `"enabled":null`, "switches[1].enabled"},
		{`"Video","Audio"`, `"Video","Video"`, "group_order[1]"},
		{`"expected_speed_mbps":1000`, `"expected_speed_mbps":-1`, "expected_speed_mbps"},
		{`"switch_id":53`, `"switch_id":99`, "watchlist[0].switch_id"},
	} {
		bad := strings.Replace(sampleProfile, tc.old, tc.bad, 1)
		for _, w := range []*httptest.ResponseRecorder{
			do(h, "POST", "/api/profiles/import/preview", bad), applyConfig(h, bad, preview),
		} {
			if w.Code != 400 || !strings.Contains(w.Body.String(), tc.field) {
				t.Errorf("%s: %d %s", tc.field, w.Code, w.Body)
			}
		}
	}
	if w := do(h, "POST", "/api/profiles/import", sampleProfile+" {}"); w.Code != 400 {
		t.Fatalf("accepted trailing JSON: %s", w.Body)
	}
	if w := do(h, "POST", "/api/mode/lock", `{"locked":true}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	for _, w := range []*httptest.ResponseRecorder{
		do(h, "POST", "/api/profiles/import/preview", sampleProfile), applyConfig(h, sampleProfile, preview),
	} {
		if w.Code != http.StatusLocked {
			t.Fatalf("read-only lock: %d %s", w.Code, w.Body)
		}
	}
	if w := do(h, "POST", "/api/mode/lock", `{"locked":false}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if w := applyConfig(h, strings.Replace(sampleProfile, "Stage", "Changed", 1), preview); w.Code != http.StatusConflict {
		t.Fatalf("allowed changing the file after preview: %d %s", w.Code, w.Body)
	}
	if _, err := s.Store.DB.Exec(`CREATE TRIGGER fail_config_port BEFORE INSERT ON ports
		BEGIN SELECT RAISE(ABORT, 'test port write failed'); END`); err != nil {
		t.Fatal(err)
	}
	if w := applyConfig(h, sampleProfile, preview); w.Code != http.StatusInternalServerError {
		t.Fatalf("write failure: %d %s", w.Code, w.Body)
	}
	profiles, _ := s.Store.Profiles()
	current, err := s.Store.ProfileConfig(ctx, pid)
	var total int
	countErr := s.Store.DB.QueryRow(`SELECT COUNT(*) FROM switches`).Scan(&total)
	if err != nil || countErr != nil || !reflect.DeepEqual(current, original) ||
		len(profiles) != 1 || !profiles[0].Active || s.Engine.ProfileID() != pid || total != len(original.Switches) {
		t.Fatalf("validation or failed write changed the database: %+v, %+v, %d, %v", profiles, current, total, err)
	}
	if err := s.Store.SaveGroupOrder(ctx, pid, []string{"Changed"}); err != nil {
		t.Fatal(err)
	}
	if w := applyConfig(h, sampleProfile, preview); w.Code != http.StatusConflict {
		t.Fatalf("stale preview should be rejected: %d %s", w.Code, w.Body)
	}
}

func TestLegacyProfileConfigImport(t *testing.T) {
	s, h := newDevicesServer(t)
	body := `{"name":"Legacy","settings":{"thresholds":{"cpu_pct":75},"alert_sound":true},
		"switches":[{"id":0,"profile_id":8,"name":"Old","ip":"192.0.2.1","snmp_port":161,"enabled":true,
		"community":"sample-old-secret","snmpv3_key":"sample-key","ssh_password":"sample-password","last_trap_ts":"2025-01-01T00:00:00Z",
		"web_url":"http://user:sample-url-secret@192.0.2.1/manage?token=sample-token"}],
		"ports":{"Old":{"2":{"excluded":true}}},"watchlist":null,"topology_layout":{},"aliases":null}`
	w := do(h, "POST", "/api/profiles/import", body)
	var result struct{ ID int64 }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.ID == s.Engine.ProfileID() {
		t.Fatalf("legacy endpoint: %d %s", w.Code, w.Body)
	}
	imported := engine.New(s.Store, result.ID, nil).Settings()
	sws, err := s.Store.Switches(result.ID)
	if err != nil || imported.Thresholds.CPUPct != 75 || imported.SetupPingIntervalSec != 60 ||
		imported.RehearsalPingIntervalSec != 60 || len(sws) != 1 || !sws[0].NeedsCredentials() ||
		sws[0].LastTrapTS != nil || sws[0].WebURL != "http://192.0.2.1/manage" {
		t.Fatalf("legacy settings=%+v switches=%+v err=%v", imported, sws, err)
	}
	ports, err := s.Store.PortSettings(sws[0].ID)
	if err != nil || !ports[2].Excluded {
		t.Fatalf("legacy ports: %v %v", ports, err)
	}
}
