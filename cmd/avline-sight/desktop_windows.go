//go:build windows

package main

// Windows のデスクトップ連携: 呼び出し元コンソールへの接続、エラーのメッセージボックス、
// 同じデータフォルダでの二重起動防止、タスクトレイのアイコンとメニュー。
// 外部ライブラリを増やさないよう、Win32 API を golang.org/x/sys/windows で直接呼ぶ。

import (
	_ "embed"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

//go:embed assets/tray.ico
var trayIcon []byte

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procAttachConsole            = kernel32.NewProc("AttachConsole")
	procGlobalAlloc              = kernel32.NewProc("GlobalAlloc")
	procGlobalLock               = kernel32.NewProc("GlobalLock")
	procGlobalUnlock             = kernel32.NewProc("GlobalUnlock")
	procGlobalFree               = kernel32.NewProc("GlobalFree")
	procRtlMoveMemory            = kernel32.NewProc("RtlMoveMemory")
	procMessageBoxW              = user32.NewProc("MessageBoxW")
	procRegisterClassExW         = user32.NewProc("RegisterClassExW")
	procUnregisterClassW         = user32.NewProc("UnregisterClassW")
	procCreateWindowExW          = user32.NewProc("CreateWindowExW")
	procDefWindowProcW           = user32.NewProc("DefWindowProcW")
	procDestroyWindow            = user32.NewProc("DestroyWindow")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procPostQuitMessage          = user32.NewProc("PostQuitMessage")
	procRegisterWindowMessageW   = user32.NewProc("RegisterWindowMessageW")
	procCreatePopupMenu          = user32.NewProc("CreatePopupMenu")
	procAppendMenuW              = user32.NewProc("AppendMenuW")
	procTrackPopupMenu           = user32.NewProc("TrackPopupMenu")
	procDestroyMenu              = user32.NewProc("DestroyMenu")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procGetCursorPos             = user32.NewProc("GetCursorPos")
	procCreateIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon              = user32.NewProc("DestroyIcon")
	procOpenClipboard            = user32.NewProc("OpenClipboard")
	procEmptyClipboard           = user32.NewProc("EmptyClipboard")
	procSetClipboardData         = user32.NewProc("SetClipboardData")
	procCloseClipboard           = user32.NewProc("CloseClipboard")
	procShellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
)

const (
	attachParentProcess = ^uint32(0) // ATTACH_PARENT_PROCESS (-1)

	mbOK          = 0x0
	mbIconError   = 0x10
	mbIconInfo    = 0x40
	mbSetForegrnd = 0x10000

	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmCommand       = 0x0111
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmApp           = 0x8000
	wmTray          = wmApp + 1

	nimAdd    = 0
	nimDelete = 2
	nifMessage = 0x1
	nifIcon    = 0x2
	nifTip     = 0x4

	mfString    = 0x0
	mfSeparator = 0x800
	tpmRightBtn = 0x2
	tpmBottom   = 0x20

	cfUnicodeText = 13
	gmemMoveable  = 0x2
)

// attachConsole は呼び出し元（PowerShell等）のコンソールにつなぐ。ダブルクリック起動では false。
// windowsgui でビルドした実行ファイルは自分のコンソールを持たないため、
// --help やエラーを呼び出し元の画面に出すにはこの接続が要る。
// 呼び出し元が出力先をファイル等へ向けている（> file）ときは、その指定を優先してつなぎ直さない。
func attachConsole() bool {
	if usableHandle(os.Stdout) && usableHandle(os.Stderr) {
		return true
	}
	if r, _, _ := procAttachConsole.Call(uintptr(attachParentProcess)); r == 0 {
		return usableHandle(os.Stderr)
	}
	for _, target := range []**os.File{&os.Stdout, &os.Stderr} {
		if usableHandle(*target) {
			continue
		}
		// 書くだけなので書込み権限で開き、親のコンソールと共有できるよう読み書きとも共有にする
		h, err := windows.CreateFile(windows.StringToUTF16Ptr("CONOUT$"), windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err == nil {
			*target = os.NewFile(uintptr(h), "CONOUT$")
		}
	}
	// エラーは標準エラーへ出すので、そこが使えないときはメッセージボックスに回す
	return usableHandle(os.Stderr)
}

// usableHandle は標準出力などが書ける先（コンソール・ファイル・パイプ）につながっているか。
func usableHandle(f *os.File) bool {
	if f == nil {
		return false
	}
	kind, err := windows.GetFileType(windows.Handle(f.Fd()))
	return err == nil && kind != windows.FILE_TYPE_UNKNOWN
}

func messageBox(title, text string, flags uintptr) {
	// 文中のNUL文字は変換できないので取り除く（nilを渡すと空の箱になる）
	t, _ := windows.UTF16PtrFromString(strings.ReplaceAll(title, "\x00", ""))
	m, _ := windows.UTF16PtrFromString(strings.ReplaceAll(text, "\x00", ""))
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), flags|mbSetForegrnd)
}

// showStartupMessage は黒い画面が無いときの起動の知らせをメッセージボックスで出す。
func showStartupMessage(text string, isError bool) {
	icon := uintptr(mbIconInfo)
	if isError {
		icon = mbIconError
	}
	messageBox("AVLine-Sight", text, mbOK|icon)
}

// acquireSingleInstance は同じデータフォルダでの二重起動を防ぐ。
// 2つが同じデータベースへ書くと監視記録が壊れうるため、別フォルダ（--data）なら同時に動かせる。
func acquireSingleInstance(dataDir string) (func(), error) {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		abs = dataDir
	}
	sum := sha256.Sum256([]byte(strings.ToLower(abs)))
	name, _ := windows.UTF16PtrFromString(`Local\AVLine-Sight-` + hex.EncodeToString(sum[:8]))
	h, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, errAlreadyRunning
	}
	if err != nil {
		return nil, fmt.Errorf("二重起動の確認に失敗しました: %w", err)
	}
	return func() { windows.CloseHandle(h) }, nil
}

// iconFromICO は埋め込んだ ICO から指定の大きさの画像を取り出してアイコンを作る。
func iconFromICO(data []byte, size int) (windows.Handle, error) {
	if len(data) < 6 {
		return 0, errors.New("アイコンのデータが短すぎます")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	best := -1
	for i := 0; i < count; i++ {
		entry := 6 + i*16
		if entry+16 > len(data) {
			break
		}
		w := int(data[entry])
		if w == 0 {
			w = 256
		}
		if best < 0 || w == size {
			best = entry
		}
	}
	if best < 0 {
		return 0, errors.New("アイコンの画像が見つかりません")
	}
	length := binary.LittleEndian.Uint32(data[best+8:])
	offset := binary.LittleEndian.Uint32(data[best+12:])
	if length == 0 || uint64(offset)+uint64(length) > uint64(len(data)) {
		return 0, errors.New("アイコンのデータが壊れています")
	}
	img := data[offset : offset+length]
	h, _, err := procCreateIconFromResourceEx.Call(uintptr(unsafe.Pointer(&img[0])), uintptr(length), 1, 0x30000, uintptr(size), uintptr(size), 0)
	if h == 0 {
		return 0, fmt.Errorf("アイコンを作れません: %w", err)
	}
	return windows.Handle(h), nil
}

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type notifyIconData struct {
	cbSize           uint32
	hWnd             windows.HWND
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            windows.Handle
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         windows.GUID
	hBalloonIcon     windows.Handle
}

type winMsg struct {
	hwnd    windows.HWND
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
	private uint32
}

// tray はタスクトレイのアイコン。メッセージは作成したスレッドに届くため、
// 1つのOSスレッドに固定したゴルーチンで作成からメッセージループまでを行う。
type tray struct {
	url            string
	onQuit         func()
	hwnd           windows.HWND
	icon           windows.Handle
	taskbarCreated uint32
	done           chan struct{}
	closeOnce      sync.Once
}

var activeTray *tray // ウィンドウプロシージャから参照する（トレイは1つだけ）

// startTray はトレイを表示する。戻り値の関数でアイコンを消してループを終える（終わるまで待つ）。
func startTray(url string, onQuit func()) (func(), error) {
	t := &tray{url: url, onQuit: onQuit, done: make(chan struct{})}
	ready := make(chan error, 1)
	go t.loop(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return func() {
		t.closeOnce.Do(func() { procPostMessageW.Call(uintptr(t.hwnd), wmClose, 0, 0) })
		<-t.done
	}, nil
}

func (t *tray) loop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(t.done)
	activeTray = t
	defer func() { activeTray = nil }()
	icon, err := iconFromICO(trayIcon, 16)
	if err != nil {
		ready <- err
		return
	}
	t.icon = icon
	defer procDestroyIcon.Call(uintptr(icon))
	className, _ := windows.UTF16PtrFromString("AVLineSightTray")
	var instance windows.Handle
	windows.GetModuleHandleEx(0, nil, &instance)
	wc := wndClassEx{lpfnWndProc: windows.NewCallback(trayWndProc), hInstance: instance, lpszClassName: className}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		ready <- fmt.Errorf("トレイ用ウィンドウを登録できません: %w", err)
		return
	}
	defer procUnregisterClassW.Call(uintptr(unsafe.Pointer(className)), uintptr(instance))
	// 見えないウィンドウ（メッセージ受信専用）。HWND_MESSAGE はトレイ再作成の通知を受けないため通常のウィンドウにする
	h, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(className)), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
	if h == 0 {
		ready <- fmt.Errorf("トレイ用ウィンドウを作れません: %w", err)
		return
	}
	t.hwnd = windows.HWND(h)
	// エクスプローラー再起動でトレイが作り直されたときに、アイコンを出し直すための通知
	tc, _ := windows.UTF16PtrFromString("TaskbarCreated")
	r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(tc)))
	t.taskbarCreated = uint32(r)
	if err := t.addIcon(); err != nil {
		procDestroyWindow.Call(h)
		// DestroyWindow が出した WM_QUIT を取り出してから戻る（スレッドに残さない）
		var m winMsg
		for {
			if r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
				break
			}
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
		ready <- err
		return
	}
	ready <- nil
	defer func() {
		// どの経路で抜けてもアイコンを残さない（WM_CLOSE で消していれば何もしない）
		d := t.iconData()
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
	}()
	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (t *tray) iconData() notifyIconData {
	d := notifyIconData{hWnd: t.hwnd, uID: 1, uFlags: nifMessage | nifIcon | nifTip, uCallbackMessage: wmTray, hIcon: t.icon}
	d.cbSize = uint32(unsafe.Sizeof(d))
	tip, _ := windows.UTF16FromString(trayTooltip(t.url))
	copy(d.szTip[:len(d.szTip)-1], tip)
	return d
}

func (t *tray) addIcon() error {
	d := t.iconData()
	if r, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&d))); r == 0 {
		return fmt.Errorf("トレイにアイコンを出せません: %w", err)
	}
	return nil
}

func (t *tray) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	for _, item := range trayMenuItems() {
		if item.id == 0 {
			procAppendMenuW.Call(menu, mfSeparator, 0, 0)
			continue
		}
		label, _ := windows.UTF16PtrFromString(item.label)
		procAppendMenuW.Call(menu, mfString, uintptr(item.id), uintptr(unsafe.Pointer(label)))
	}
	var pt struct{ x, y int32 }
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	// 前面に出さないと、メニューの外をクリックしても閉じない（Windowsの既知の作法）
	procSetForegroundWindow.Call(uintptr(t.hwnd))
	procTrackPopupMenu.Call(menu, tpmRightBtn|tpmBottom, uintptr(pt.x), uintptr(pt.y), 0, uintptr(t.hwnd), 0)
	procPostMessageW.Call(uintptr(t.hwnd), 0, 0, 0)
}

func (t *tray) command(id int) {
	switch id {
	case menuOpen:
		openBrowser(t.url)
	case menuCopy:
		if err := copyToClipboard(t.hwnd, t.url); err != nil {
			slog.Warn("URLをコピーできませんでした", "err", err)
		}
	case menuQuit:
		t.onQuit()
	}
}

func trayWndProc(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	t := activeTray
	switch {
	case t != nil && msg == wmTray:
		switch uint32(lParam) {
		case wmLButtonDblClk:
			openBrowser(t.url)
		case wmRButtonUp:
			t.showMenu()
		}
		return 0
	case t != nil && msg == wmCommand:
		t.command(int(wParam & 0xffff))
		return 0
	case t != nil && t.taskbarCreated != 0 && msg == t.taskbarCreated:
		if err := t.addIcon(); err != nil {
			slog.Warn("トレイのアイコンを出し直せませんでした", "err", err)
		}
		return 0
	case msg == wmClose:
		if t != nil {
			d := t.iconData()
			procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&d)))
			// 外から閉じられたときもアイコンだけ消えて本体が残らないよう、終了と同じ扱いにする（二度呼んでも害はない）
			t.onQuit()
		}
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case msg == wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

func copyToClipboard(owner windows.HWND, text string) error {
	u, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	// 所有者のウィンドウを渡さないと、EmptyClipboard 後の SetClipboardData が失敗する（Win32 の仕様）
	if r, _, err := procOpenClipboard.Call(uintptr(owner)); r == 0 {
		return fmt.Errorf("クリップボードを開けません: %w", err)
	}
	defer procCloseClipboard.Call()
	if r, _, err := procEmptyClipboard.Call(); r == 0 {
		return fmt.Errorf("クリップボードを空にできません: %w", err)
	}
	size := uintptr(len(u) * 2)
	mem, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if mem == 0 {
		return fmt.Errorf("メモリを確保できません: %w", err)
	}
	p, _, err := procGlobalLock.Call(mem)
	if p == 0 {
		procGlobalFree.Call(mem)
		return fmt.Errorf("メモリを固定できません: %w", err)
	}
	procRtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&u[0])), size)
	procGlobalUnlock.Call(mem)
	if r, _, err := procSetClipboardData.Call(cfUnicodeText, mem); r == 0 {
		// 渡せなかったときだけこちらで解放する（成功時はクリップボードが持つ）
		procGlobalFree.Call(mem)
		return fmt.Errorf("クリップボードに書けません: %w", err)
	}
	return nil
}
