package term_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/sim"
	"github.com/Mono-Gen/avline-sight/internal/term"
)

// 実機の代わりにラボのモックCLIへ Telnet/SSH で接続し、ブリッジ経由でコマンド応答が得られることを確かめる。
const (
	testSNMPBase = 28100
	testCLIBase  = 28300
	testSSHBase  = 28400
)

// 停止は非同期でポート解放を待てないため、ラボはテスト間で1つを共有する（プロセス終了で停止）。
var (
	labOnce sync.Once
	labErr  error
)

func startLab(t *testing.T) {
	t.Helper()
	labOnce.Do(func() {
		lab := sim.NewLab(1, 1)
		labErr = lab.Start(context.Background(), sim.Config{BaseSNMPPort: testSNMPBase, BaseCLIPort: testCLIBase, BaseSSHPort: testSSHBase})
	})
	if labErr != nil {
		t.Fatal(labErr)
	}
}

// readUntil は want が現れるまで読み続ける（タイムアウト付き）。
func readUntil(t *testing.T, s term.Session, want string) string {
	t.Helper()
	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		p := make([]byte, 4096)
		for !strings.Contains(buf.String(), want) {
			n, err := s.Read(p)
			buf.Write(p[:n])
			if err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.Close()
		<-done
	}
	if !strings.Contains(buf.String(), want) {
		t.Fatalf("%q が出力に現れません: %q", want, buf.String())
	}
	return buf.String()
}

func TestSSHBridgeToLab(t *testing.T) {
	startLab(t)
	s, fp, err := term.Dial(context.Background(), "ssh", "127.0.0.1", testSSHBase, "admin", "x", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("ホスト鍵指紋が返りません: %q", fp)
	}
	readUntil(t, s, "#")
	if err := s.Resize(120, 40); err != nil {
		t.Fatalf("リサイズ失敗: %v", err)
	}
	// 1文字ずつ送り、Backspaceで誤入力を消せること（PTYの行編集）を確かめる
	s.Write([]byte("show poX\x7fe\r"))
	out := readUntil(t, s, "Power budget")
	if !strings.Contains(out, "show poX\b \be") {
		t.Fatalf("エコーに行編集（Backspace）が反映されていません: %q", out)
	}
	if strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Fatalf("改行がCRLFになっていません（端末で階段表示になる）: %q", out)
	}
	s.Write([]byte("exit\r"))
	readUntil(t, s, "Goodbye")
}

func TestTelnetBridgeToLab(t *testing.T) {
	startLab(t)
	s, _, err := term.Dial(context.Background(), "telnet", "127.0.0.1", testCLIBase, "", "", 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	readUntil(t, s, "User:")
	s.Write([]byte("admin\r\n"))
	readUntil(t, s, "Password:")
	s.Write([]byte("x\r\n"))
	readUntil(t, s, "#")
	s.Write([]byte("show poe\r\n"))
	readUntil(t, s, "Power budget")
}
