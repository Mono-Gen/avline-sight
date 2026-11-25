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

// icoWith は指定の幅の画像（中身は幅の1バイト）を並べたICOを作る。
func icoWith(widths ...int) []byte {
	head := []byte{0, 0, 1, 0, byte(len(widths)), 0}
	var body []byte
	offset := 6 + 16*len(widths)
	for _, w := range widths {
		side := byte(w)
		if w == 256 {
			side = 0
		}
		entry := make([]byte, 16)
		entry[0], entry[1] = side, side
		entry[8] = 1 // 長さ1
		pos := offset + len(body)
		entry[12], entry[13] = byte(pos), byte(pos>>8)
		head = append(head, entry...)
		body = append(body, byte(w))
	}
	return append(head, body...)
}

// 同じ大きさがあればそれ、無ければ大きい中で最小、それも無ければ最大を選ぶ。
func TestPickICOImageChoosesSize(t *testing.T) {
	ico := icoWith(16, 24, 32, 48, 256)
	for size, want := range map[int]byte{16: 16, 20: 24, 32: 32, 40: 48, 64: 0 /* 256 */, 300: 0} {
		img, err := pickICOImage(ico, size)
		if err != nil || len(img) != 1 || img[0] != want {
			t.Fatalf("size %d: img=%v err=%v want %d", size, img, err, want)
		}
	}
	if img, _ := pickICOImage(icoWith(16, 24), 64); img[0] != 24 {
		t.Fatalf("largest fallback: %v", img)
	}
}

// 壊れたICOはパニックせずエラーにする。
func TestPickICOImageRejectsBrokenData(t *testing.T) {
	outOfRange := icoWith(16)
	outOfRange[18], outOfRange[19] = 0xff, 0xff // 画像の位置をデータの外へ
	zeroLength := icoWith(16)
	zeroLength[14] = 0 // 画像の長さを0に
	for _, data := range [][]byte{nil, {0, 0, 1, 0}, outOfRange, zeroLength} {
		if _, err := pickICOImage(data, 16); err == nil {
			t.Fatalf("accepted broken data %v", data)
		}
	}
}
