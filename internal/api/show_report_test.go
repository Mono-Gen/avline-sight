package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func TestChecklistGatePersistsOneRemainder(t *testing.T) {
	for _, phase := range []string{"setup", "rehearsal"} {
		t.Run(phase, func(t *testing.T) { testChecklistGate(t, phase) })
	}
}

func testChecklistGate(t *testing.T, phase string) {
	t.Helper()
	s, h := newDevicesServer(t)
	if w := do(h, "PUT", "/api/phase", `{"phase":"`+phase+`"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	var checklist engine.ShowChecklist
	if w := do(h, "GET", "/api/checklist/pre-show", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &checklist) != nil || len(checklist.Items) != 15 {
		t.Fatalf("checklist response %d: %s", w.Code, w.Body)
	}
	if w := do(h, "PUT", "/api/phase", `{"phase":"show"}`); w.Code != 409 || s.Engine.Phase() != phase {
		t.Fatalf("unconfirmed phase: %d %s", w.Code, w.Body)
	}
	if _, err := s.Store.DB.Exec(`CREATE TRIGGER reject_remainder BEFORE INSERT ON events WHEN NEW.code='PRE_SHOW_CHECKLIST_REMAINDER' BEGIN SELECT RAISE(ABORT,'write failed'); END`); err != nil {
		t.Fatal(err)
	}
	const confirm = `{"phase":"show","confirm_checklist":true}`
	if w := do(h, "PUT", "/api/phase", confirm); w.Code != 500 || s.Engine.Phase() != phase {
		t.Fatalf("phase changed despite failed log write: %d %s", w.Code, w.Body)
	}
	if baselines, err := s.Store.Baselines(s.Engine.ProfileID()); err != nil || len(baselines) != 0 {
		t.Fatalf("phase baseline was not rolled back: %v (%v)", baselines, err)
	}
	if _, err := s.Store.DB.Exec(`DROP TRIGGER reject_remainder`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if w := do(h, "PUT", "/api/phase", confirm); w.Code != 200 || s.Engine.Phase() != "show" {
			t.Fatalf("confirmed phase: %d %s", w.Code, w.Body)
		}
	}
	events, err := s.Store.Events(store.EventFilter{Category: "SYSTEM"})
	remaining := 0
	for _, event := range events {
		if event.Code != "PRE_SHOW_CHECKLIST_REMAINDER" {
			continue
		}
		remaining++
		items, ok := event.Detail["items"].([]any)
		if !ok || len(items) == 0 {
			t.Fatalf("missing detailed remainder: %v", event.Detail)
		}
		for _, value := range items {
			if value.(map[string]any)["state"] == "pass" {
				t.Fatalf("passed item in remainder: %v", value)
			}
		}
	}
	if err != nil || remaining != 1 {
		t.Fatalf("wanted one remainder event, got %d (%v)", remaining, err)
	}
	baselines, err := s.Store.Baselines(s.Engine.ProfileID())
	if err != nil || len(baselines) != 1 {
		t.Fatalf("baseline %v (%v)", baselines, err)
	}
	rows := showReportRows(t, do(h, "GET", "/api/export/show-report.csv", "").Body.String())
	if len(rows) != 3 || rows[1][1] != "phase" || rows[2][1] != "checklist" || !strings.Contains(rows[2][9], `"items"`) {
		t.Fatalf("latest report missing phase and remainder: %v", rows)
	}
}

func TestShowReportCSVRangeAndEscaping(t *testing.T) {
	s, h := newDevicesServer(t)
	base := time.Date(2024, 1, 2, 16, 0, 0, 0, time.UTC)
	add := func(offset int, code, category, severity, title string, detail map[string]any) model.Event {
		t.Helper()
		ev := model.Event{TS: base.Add(time.Duration(offset) * time.Minute), Code: code, Category: category, Severity: severity, Title: title, Detail: detail}
		if inserted, err := s.Store.InsertEvent(&ev); !inserted || err != nil {
			t.Fatalf("insert %s: %v", code, err)
		}
		return ev
	}
	phase := func(offset int, value string) {
		add(offset, "PHASE_CHANGED", "SYSTEM", "INFO", value, map[string]any{"phase": value})
	}
	phase(-30, "show")
	phase(-20, "setup")
	old := add(-1, "OLD_ALERT", "ANOMALY", "WARNING", "Before start", nil)
	phase(0, "show")
	alert := add(1, "PHY_ERRORS", "ANOMALY", "CRITICAL", "  =SUM(1,2)", nil)
	add(2, "MARKER", "MARKER", "INFO", "\t=1,\nマーカー \"A\"", nil)
	add(2, "SYSLOG", "SYSLOG", "WARNING", "Raw log", nil)
	add(2, "EXCLUDED", "ANOMALY", "WARNING", "Suppressed", map[string]any{"_suppressed": true})
	phase(5, "rehearsal")
	add(6, "MARKER", "MARKER", "INFO", "After show", nil)
	if _, err := s.Store.DB.Exec("UPDATE events SET cleared_at=? WHERE id=?", base.Add(3*time.Minute).UnixMilli(), old.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB.Exec("UPDATE events SET acked=1,acked_at=?,cleared_at=? WHERE id=?",
		base.Add(3*time.Minute).UnixMilli(), base.Add(4*time.Minute).UnixMilli(), alert.ID); err != nil {
		t.Fatal(err)
	}
	w := do(h, "GET", "/api/export/show-report.csv", "")
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download: %d %v", w.Code, w.Header())
	}
	rows := showReportRows(t, w.Body.String())
	wantKinds := []string{"type", "phase", "alert", "marker", "recovery", "ack", "recovery", "phase"}
	if len(rows) != len(wantKinds) {
		t.Fatalf("unexpected timeline: %v", rows)
	}
	for i, kind := range wantKinds {
		if rows[i][1] != kind {
			t.Errorf("row %d: %v, want %s", i, rows[i], kind)
		}
	}
	if rows[0][0] != "time (JST)" || rows[1][0] != "2024-01-03 01:00:00.000" || rows[2][8] != "'  =SUM(1,2)" ||
		rows[3][8] != "'\t=1,\nマーカー \"A\"" || rows[4][3] != "OLD_ALERT" {
		t.Fatalf("wrong JST, escaping, or carry-over recovery: %v", rows)
	}
	params := url.Values{"from": {base.Add(5 * time.Minute).Format(time.RFC3339)}, "to": {base.Add(6 * time.Minute).Format(time.RFC3339)}}
	w = do(h, "GET", "/api/export/show-report.csv?"+params.Encode(), "")
	rows = showReportRows(t, w.Body.String())
	if w.Code != 200 || len(rows) != 3 || rows[2][8] != "After show" {
		t.Fatalf("custom range: %d %v", w.Code, rows)
	}
	if w := do(h, "GET", "/api/export/show-report.csv?from=invalid", ""); w.Code != 400 {
		t.Fatal(w.Code)
	}
}

func showReportRows(t *testing.T, body string) [][]string {
	t.Helper()
	text, bom := strings.CutPrefix(body, "\uFEFF")
	rows, err := csv.NewReader(strings.NewReader(text)).ReadAll()
	if !bom || err != nil {
		t.Fatalf("invalid CSV: bom=%v err=%v %q", bom, err, body)
	}
	return rows
}

func TestShowReportOmitsFalseRecoveries(t *testing.T) {
	s, h := newDevicesServer(t)
	at := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		code    string
		oneShot bool
		delay   time.Duration
	}{
		{"ONE_SHOT", true, time.Second},
		{"LINK_FLAP", false, time.Second},
		{"SAME_TIME", false, 0},
		{"BEFORE_EVENT", false, -time.Second},
		{"NORMAL", false, time.Second},
	} {
		cleared := at.Add(tc.delay)
		event := model.Event{TS: at, Code: tc.code, Category: "ANOMALY", Severity: model.SevWarning,
			ClearedAt: &cleared, Detail: map[string]any{"_one_shot": tc.oneShot}}
		if inserted, err := s.Store.InsertEvent(&event); !inserted || err != nil {
			t.Fatalf("insert %s: %v", tc.code, err)
		}
	}
	w := do(h, "GET", showReportTestURL(at), "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	rows := showReportRows(t, w.Body.String())
	if len(rows) != 7 || rows[6][1] != "recovery" || rows[6][3] != "NORMAL" {
		t.Fatalf("unexpected timeline: %v", rows)
	}
}

func TestShowReportReleasesDBAndRejectsOversizeBeforeCSV(t *testing.T) {
	s, h := newDevicesServer(t)
	at := time.Now().UTC().Truncate(time.Second)
	event := model.Event{TS: at, Code: "MARKER", Category: "MARKER", Title: "test"}
	if inserted, err := s.Store.InsertEvent(&event); !inserted || err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	err := writeShowReport(t.Context(), &body, s.Store, store.ShowReportRange{From: at.Add(-time.Second), To: at.Add(time.Second)}, func() {
		if got := s.Store.DB.Stats().InUse; got != 0 {
			t.Errorf("DB still holds %d connections at the start of export", got)
		}
	})
	if err != nil || !strings.Contains(body.String(), "MARKER") {
		t.Fatal(err, body.String())
	}
	if _, err := s.Store.DB.Exec(`WITH RECURSIVE n(i) AS (VALUES(1) UNION ALL SELECT i+1 FROM n WHERE i<10000)
		INSERT INTO events(ts,category,code,severity,title,detail_json)
		SELECT ?, 'MARKER', 'MARKER', 'INFO', 'entry', '{}' FROM n`, at.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	w := do(h, "GET", showReportTestURL(at), "")
	if w.Code != 422 || w.Header().Get("Content-Disposition") != "" || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("oversize export: %d %v %q", w.Code, w.Header(), w.Body.String())
	}
}

func showReportTestURL(at time.Time) string {
	query := url.Values{"from": {at.Add(-time.Minute).Format(time.RFC3339)}, "to": {at.Add(time.Minute).Format(time.RFC3339)}}
	return "/api/export/show-report.csv?" + query.Encode()
}
