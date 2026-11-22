package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

const defaultConfigName = "avline-sight.config.json"

func configFlag(flags *flag.FlagSet) *string {
	return flags.String("config", "", "起動時に読み込む設定ファイル `file.json`（--config file.json、既存データがあっても適用）")
}

func loadStartupConfig(ctx context.Context, st *store.Store, exeDirectory string, explicit string) (string, error) {
	path := explicit
	if path == "" {
		exists, err := st.HasProfiles(ctx)
		if err != nil {
			return "", fmt.Errorf("初回起動の確認: %w", err)
		}
		if exists {
			return "", nil
		}
		path = filepath.Join(exeDirectory, defaultConfigName)
	}
	f, err := os.Open(path)
	if err != nil {
		if explicit == "" && errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return startupConfigFailure(explicit, err)
	}
	defer f.Close()
	c, err := engine.ReadProfileConfig(f)
	if err != nil {
		return startupConfigFailure(explicit, err)
	}
	opts := store.ConfigApply{Activate: true, OnlyEmpty: explicit == "", ReuseActive: explicit != ""}
	if explicit == "" {
		opts.StartupEvent = &model.Event{TS: time.Now(), Category: "SYSTEM", Severity: model.SevInfo,
			Code: "CONFIG_AUTO_IMPORTED", Title: "設定ファイルを自動で読み込みました"}
	}
	if _, err := st.ApplyProfileConfig(ctx, c, opts); err != nil {
		return startupConfigFailure(explicit, err)
	}
	return "", nil
}

func startupConfigFailure(explicit string, err error) (string, error) {
	if explicit != "" {
		return "", fmt.Errorf("--config %q: %w", explicit, err)
	}
	return err.Error(), nil
}
