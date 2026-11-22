package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Mono-Gen/avline-sight/internal/engine"
)

func TestCleanupSettingsAPIAndRemovedOperations(t *testing.T) {
	s, _ := newDevicesServer(t)
	// 廃止APIがSPAのindex.htmlへフォールバックして成功に見えないことも確認する。
	s.Static = fstest.MapFS{"index.html": {Data: []byte("test UI")}}
	h := s.Handler()
	for _, path := range []string{"/api/work", "/api/work/*", "/api/work/"} {
		for _, method := range []string{"POST", "DELETE"} {
			if w := do(h, method, path, `{"group":"*","minutes":15}`); w.Code != 404 {
				t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
			}
		}
	}
	if w := do(h, "PUT", "/api/phase", `{"phase":"teardown"}`); w.Code != 400 {
		t.Fatal(w.Body)
	}
	for _, field := range []string{"setup_ping_interval_sec", "rehearsal_ping_interval_sec"} {
		for _, value := range []string{"0", "29", "30", "3600", "3601", "30.5"} {
			w := do(h, "PUT", "/api/settings", fmt.Sprintf(`{"%s":%s}`, field, value))
			want := 400
			if value == "30" || value == "3600" {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("%s=%s: %d %s", field, value, w.Code, w.Body)
			}
		}
	}
	w := do(h, "GET", "/api/settings", "")
	if strings.Contains(w.Body.String(), "alert_sound") {
		t.Fatal("廃止した音声設定が残る")
	}
	var settings engine.Settings
	if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil || settings.SetupPingIntervalSec != 60 || settings.RehearsalPingIntervalSec != 3600 {
		t.Fatalf("%s %v", w.Body, err)
	}
	w = do(h, "GET", "/api/groups", "")
	if strings.Contains(w.Body.String(), "work_until") {
		t.Fatal("作業期限が残る")
	}
	if _, err := s.Store.DB.Exec(`INSERT OR REPLACE INTO meta(k,v) VALUES('work_mode','invalid legacy JSON')`); err != nil {
		t.Fatal(err)
	}
	if w := do(h, "PUT", "/api/groups/", `{"new_name":"Audio"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
}
