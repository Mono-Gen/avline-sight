package main

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func startupStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "config.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

const startupConfig = `{"name":"Tour","settings":{"setup_ping_interval_sec":90},"switches":[{"name":"Core","ip":"192.0.2.1","enabled":true}]}`

func TestAutomaticProfileConfigOnlyOnFirstStart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, defaultConfigName)
	st := startupStore(t)
	if warning, err := loadStartupConfig(ctx, st, dir, ""); warning != "" || err != nil {
		t.Fatalf("absent file: %q %v", warning, err)
	}
	if err := os.WriteFile(path, []byte(startupConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if warning, err := loadStartupConfig(ctx, st, dir, ""); warning != "" || err != nil {
		t.Fatalf("first start: %q %v", warning, err)
	}
	pid, err := st.EnsureDefaultProfile()
	if err != nil || engine.New(st, pid, nil).Settings().SetupPingIntervalSec != 90 {
		t.Fatalf("active imported settings: %d %v", pid, err)
	}
	if err := os.WriteFile(path, []byte(`{"name":"Different"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if warning, err := loadStartupConfig(ctx, st, dir, ""); warning != "" || err != nil {
		t.Fatalf("second start: %q %v", warning, err)
	}
	profiles, err := st.Profiles()
	if err != nil || len(profiles) != 1 || profiles[0].Name != "Tour" || !profiles[0].Active {
		t.Fatalf("autostart replaced an existing profile: %v %v", profiles, err)
	}
	events, err := st.Events(store.EventFilter{ProfileID: pid, Category: "SYSTEM"})
	if err != nil || len(events) != 1 || events[0].Code != "CONFIG_AUTO_IMPORTED" || events[0].Severity != model.SevInfo {
		t.Fatalf("automatic import INFO event: %v %v", events, err)
	}
}

func TestStartupConfigInvalidAndExplicitOverride(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	badPath := filepath.Join(dir, defaultConfigName)
	if err := os.WriteFile(badPath, []byte(`{"name":"Invalid","settings":{"rehearsal_ping_interval_sec":10}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	st := startupStore(t)
	if reason, err := loadStartupConfig(ctx, st, dir, ""); err != nil || !strings.Contains(reason, "rehearsal_ping_interval_sec") {
		t.Fatalf("automatic invalid config did not return a UI warning: %q %v", reason, err)
	}
	if profiles, _ := st.Profiles(); len(profiles) != 0 {
		t.Fatal(profiles)
	}
	original, err := st.EnsureDefaultProfile()
	if err != nil {
		t.Fatal(err)
	}
	if reason, err := loadStartupConfig(ctx, st, dir, badPath); reason != "" || err == nil ||
		!strings.Contains(err.Error(), "--config") || !strings.Contains(err.Error(), "rehearsal_ping_interval_sec") {
		t.Fatalf("explicit invalid config did not fail startup: %q %v", reason, err)
	}
	explicitPath := filepath.Join(dir, "explicit.json")
	if err := os.WriteFile(explicitPath, []byte(startupConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := flag.NewFlagSet("avline-sight", flag.ContinueOnError)
	var usage bytes.Buffer
	flags.SetOutput(&usage)
	explicit := configFlag(flags)
	printUsage(flags)
	if !strings.Contains(usage.String(), "--config file.json") {
		t.Fatal(usage.String())
	}
	if err := flags.Parse([]string{"--config", explicitPath}); err != nil {
		t.Fatal(err)
	}
	if reason, err := loadStartupConfig(ctx, st, dir, *explicit); reason != "" || err != nil {
		t.Fatalf("explicit startup: %q %v", reason, err)
	}
	pid, err := st.EnsureDefaultProfile()
	if err != nil || pid == original || engine.New(st, pid, nil).Settings().SetupPingIntervalSec != 90 {
		t.Fatalf("explicit config did not override the active profile: %d %v", pid, err)
	}
	sws, err := st.Switches(pid)
	if err != nil || len(sws) != 1 || !sws[0].NeedsCredentials() {
		t.Fatalf("startup credentials: %v %v", sws, err)
	}
	sw := sws[0]
	sw.Community = "sample-secret"
	if err := st.SaveSwitch(&sw); err != nil {
		t.Fatal(err)
	}
	if reason, err := loadStartupConfig(ctx, st, dir, *explicit); reason != "" || err != nil {
		t.Fatalf("repeat explicit startup: %q %v", reason, err)
	}
	if active, _ := st.EnsureDefaultProfile(); active != pid {
		t.Fatal("restarting with the same file discarded credentials")
	}
	if events, err := st.Events(store.EventFilter{Category: "SYSTEM"}); err != nil || len(events) != 0 {
		t.Fatalf("invalid/explicit configs should not emit automatic success: %v %v", events, err)
	}
}
