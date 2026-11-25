package main

import (
	"errors"
	"strings"
)

// errAlreadyRunning は同じデータフォルダで既に起動しているときのエラー。
var errAlreadyRunning = errors.New("AVLine-Sight はすでに起動しています")

// トレイメニューの項目番号（0 は区切り線に使うので避ける）。
const (
	menuOpen = 1
	menuCopy = 2
	menuQuit = 3
)

type trayMenuItem struct {
	id    int    // 0 は区切り線
	label string // メニューの文言（Windowsの画面に出るため日本語）
}

// trayMenuItems はトレイの右クリックメニュー。終了は誤って押さないよう区切りの下に置く。
func trayMenuItems() []trayMenuItem {
	return []trayMenuItem{
		{id: menuOpen, label: "画面を開く"},
		{id: menuCopy, label: "URLをコピー"},
		{},
		{id: menuQuit, label: "終了"},
	}
}

// trayTooltip はアイコンに重ねたときの説明。Windowsの上限（127文字）を超えないよう切る。
func trayTooltip(url string) string {
	tip := "AVLine-Sight " + version + "\n" + url
	if r := []rune(tip); len(r) > 127 {
		tip = string(r[:127])
	}
	return tip
}

// startupErrorText は起動失敗の知らせ。黒い画面が無いときはメッセージボックスに出すので、
// 何が起きたかと、よくある原因への対処を1つにまとめる。
func startupErrorText(err error, logPath string) string {
	var b strings.Builder
	if errors.Is(err, errAlreadyRunning) {
		b.WriteString("AVLine-Sight はすでに起動しています。\n画面右下のタスクトレイのアイコンから画面を開けます。")
		return b.String()
	}
	b.WriteString("起動に失敗しました。\n\n")
	b.WriteString(err.Error())
	if logPath != "" {
		b.WriteString("\n\n詳しい記録: ")
		b.WriteString(logPath)
	}
	return b.String()
}
