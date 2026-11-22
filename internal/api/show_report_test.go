package api

import (
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
	s, h := newDevicesServer(t)
	if w := do(h, "PUT", "/api/phase", `{"phase":"rehearsal"}`); w.Code != 200 {
		t.Fatal(w.Body)
	}
	var checklist engine.ShowChecklist
	if w := do(h, "GET", "/api/checklist/pre-show", ""); w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &checklist) != nil || len(checklist.Items) != 15 {
		t.Fatalf("checklist response %d: %s", w.Code, w.Body)
	}
	if w := do(h, "PUT", "/api/phase", `{"phase":"show"}`); w.Code != 409 || s.Engine.Phase() != "rehearsal" {
		t.Fatalf("unconfirmed phase: %d %s", w.Code, w.Body)
	}
	if _, err := s.Store.DB.Exec(`CREATE TRIGGER reject_remainder BEFORE INSERT ON events WHEN NEW.code='PRE_SHOW_CHECKLIST_REMAINDER' BEGIN SELECT RAISE(ABORT,'write failed'); END`); err != nil {
		t.Fatal(err)
	}
	const confirm = `{"phase":"show","confirm_checklist":true}`
	if w := do(h, "PUT", "/api/phase", confirm); w.Code != 500 || s.Engine.Phase() != "rehearsal" {
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
