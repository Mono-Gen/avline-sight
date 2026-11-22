package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func TestPlannedIPMigrationAndReadOnlyLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	s, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := s.EnsureDefaultProfile()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAlias(pid, Alias{MatchType: "mac", MatchKey: "00:11:22:33:44:55", Label: "卓", Note: "旧形式"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`ALTER TABLE device_aliases DROP COLUMN planned_ip`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := ro.Aliases(pid)
	if err != nil || len(aliases) != 1 || aliases[0].PlannedIP != "" || aliases[0].Note != "旧形式" {
		t.Fatalf("旧再生: %+v %v", aliases, err)
	}
	if err := ro.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	aliases, err = s.Aliases(pid)
	if err != nil || len(aliases) != 1 || aliases[0].PlannedIP != "" {
		t.Fatalf("移行: %+v %v", aliases, err)
	}
	a := aliases[0]
	a.PlannedIP = "192.0.2.10"
	res, err := s.ApplyMACAliases(pid, []Alias{a}, false, true)
	if err != nil || res.Updated != 1 {
		t.Fatalf("予定IPのみ更新: %+v %v", res, err)
	}
	aliases, err = s.Aliases(pid)
	if err != nil || aliases[0].PlannedIP != "192.0.2.10" {
		t.Fatal("予定IP未保存")
	}
	res, err = s.ApplyMACAliases(pid, []Alias{a}, false, false)
	if err != nil || res.Unchanged != 1 {
		t.Fatalf("同じ予定IP: %+v %v", res, err)
	}
	sw := model.SwitchConfig{ProfileID: pid, Name: "SW", Enabled: true}
	if err := s.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	s.UpsertBinding(sw.ID, 7, model.Device{MAC: a.MatchKey, IP: "192.0.2.11"}, time.Now().Add(-time.Second))
	last, err := s.LastSeenByMAC(pid, time.Now())
	if err != nil || last[a.MatchKey].SwitchID != sw.ID || last[a.MatchKey].IfIndex != 7 {
		t.Fatalf("最終所在: %+v %v", last, err)
	}
}
