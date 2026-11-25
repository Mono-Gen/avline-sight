package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// 二重起動は失敗ではなく案内として出し、トレイから開けることを伝える。
func TestStartupErrorTextForSecondInstance(t *testing.T) {
	text := startupErrorText(fmt.Errorf("wrapped: %w", errAlreadyRunning), "data/avline-sight.log")
	if !strings.Contains(text, "すでに起動") || !strings.Contains(text, "タスクトレイ") {
		t.Fatalf("text=%q", text)
	}
	if strings.Contains(text, "起動に失敗") || strings.Contains(text, "avline-sight.log") {
		t.Fatalf("second instance must not read as a failure: %q", text)
	}
}

// 起動失敗では原因とログの場所を出す。ログを開けなかったときは場所を出さない。
func TestStartupErrorTextShowsCauseAndLog(t *testing.T) {
	text := startupErrorText(errors.New("ポート 8080 は使用中"), `C:\app\data\avline-sight.log`)
	for _, want := range []string{"起動に失敗しました", "ポート 8080 は使用中", `C:\app\data\avline-sight.log`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(startupErrorText(errors.New("x"), ""), "詳しい記録") {
		t.Fatal("log hint shown without a log file")
	}
}

// 終了は区切り線の下の最後に置き、項目番号は重ならない。
func TestTrayMenuPutsQuitLastAfterSeparator(t *testing.T) {
	items := trayMenuItems()
	if len(items) < 2 || items[len(items)-1].id != menuQuit || items[len(items)-2].id != 0 {
		t.Fatalf("items=%+v", items)
	}
	seen := map[int]bool{}
	for _, item := range items {
		if item.id != 0 && seen[item.id] {
			t.Fatalf("duplicate id %d", item.id)
		}
		seen[item.id] = true
	}
}

// ツールチップはWindowsの上限127文字に収める。
func TestTrayTooltipFitsWindowsLimit(t *testing.T) {
	if tip := trayTooltip("http://127.0.0.1:8080"); !strings.Contains(tip, "http://127.0.0.1:8080") {
		t.Fatalf("tip=%q", tip)
	}
	if n := len([]rune(trayTooltip("http://" + strings.Repeat("a", 300)))); n > 127 {
		t.Fatalf("tooltip has %d characters", n)
	}
}
