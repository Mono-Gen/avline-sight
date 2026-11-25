package main

import (
	"encoding/binary"
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

// pickICOImage は ICO の中から指定の大きさの画像を取り出す。同じ大きさが無ければ、
// それより大きい中で最小のもの（縮小のほうが拡大より粗くなりにくい）、それも無ければ最大のものを選ぶ。
func pickICOImage(data []byte, size int) ([]byte, error) {
	if len(data) < 6 {
		return nil, errors.New("アイコンのデータが短すぎます")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	best, bestWidth := -1, 0
	better := func(w int) bool {
		switch {
		case best < 0:
			return true
		case bestWidth == size:
			return false
		case w == size:
			return true
		case w > size && (bestWidth < size || w < bestWidth):
			return true
		case w < size && bestWidth < size && w > bestWidth:
			return true
		}
		return false
	}
	for i := 0; i < count; i++ {
		entry := 6 + i*16
		if entry+16 > len(data) {
			break
		}
		w := int(data[entry])
		if w == 0 {
			w = 256 // ICO では 256 を 0 と書く
		}
		if better(w) {
			best, bestWidth = entry, w
		}
	}
	if best < 0 {
		return nil, errors.New("アイコンの画像が見つかりません")
	}
	length := binary.LittleEndian.Uint32(data[best+8:])
	offset := binary.LittleEndian.Uint32(data[best+12:])
	if length == 0 || uint64(offset)+uint64(length) > uint64(len(data)) {
		return nil, errors.New("アイコンのデータが壊れています")
	}
	return data[offset : offset+length], nil
}
