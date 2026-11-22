package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func TestClearedAtMigrationAndReadOnlyLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	s, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	ev := model.Event{TS: time.Now().UTC().Add(-time.Hour), Category: "ANOMALY", Severity: model.SevWarning, Code: "LEGACY", Title: "旧障害"}
	if _, err := s.InsertEvent(&ev); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`ALTER TABLE events DROP COLUMN cleared_at`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	list, readErr := ro.Events(EventFilter{})
	if err := ro.Close(); err != nil {
		t.Fatal(err)
	}
	if readErr != nil || len(list) != 1 || list[0].ClearedAt != nil {
		t.Fatalf("旧版の読取専用再生: %+v %v", list, readErr)
	}
	s, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('events') WHERE name='cleared_at' AND "notnull"=0`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("nullable列の自動移行: %d %v", count, err)
	}
	list, err = s.Events(EventFilter{})
	if err != nil || len(list) != 1 || list[0].ClearedAt != nil {
		t.Fatalf("旧復旧時刻を捏造しない: %+v %v", list, err)
	}
	cleared := ev.TS.Add(30 * time.Minute).Truncate(time.Millisecond)
	ev.Code, ev.ClearedAt = "RECOVERED", &cleared
	if _, err := s.InsertEvent(&ev); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	list, err = s.Events(EventFilter{})
	if err != nil || len(list) != 2 || list[0].ClearedAt == nil || !list[0].ClearedAt.Equal(cleared) {
		t.Fatalf("再度の移行後も実際の復旧時刻を保持: %+v %v", list, err)
	}
}
