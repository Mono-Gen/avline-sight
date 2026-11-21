// Package term は内蔵ブラウザターミナル用のTelnet/SSHブリッジ。
// 認証情報は接続時の引数としてのみ受け取り、保存・ログ出力しない（仕様3.9③-3）。
package term

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// Session は端末セッション（Telnet/SSH共通）。
type Session interface {
	io.ReadWriteCloser
	Resize(cols, rows int) error
}

// Dial は proto("telnet"/"ssh") で host:port へ接続する。
func Dial(ctx context.Context, proto, host string, port int, user, pass string, cols, rows int) (Session, string, error) {
	addr := net.JoinHostPort(host, fmt.Sprint(port))
	switch proto {
	case "ssh":
		return dialSSH(addr, user, pass, cols, rows)
	case "telnet", "":
		d := net.Dialer{Timeout: 5 * time.Second}
		c, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil, "", fmt.Errorf("Telnet接続に失敗 %s: %w", addr, err)
		}
		t := &telnet{c: c, cols: cols, rows: rows}
		// ログインプロンプトへ自動でユーザー名・パスワードは送らない（実機のプロンプト差異に依存しないため、利用者が入力）
		_ = user
		_ = pass
		return t, "", nil
	}
	return nil, "", fmt.Errorf("未対応のプロトコル %q", proto)
}

// ---- SSH ----

type sshSession struct {
	cl  *ssh.Client
	s   *ssh.Session
	in  io.WriteCloser
	out io.Reader
}

func dialSSH(addr, user, pass string, cols, rows int) (Session, string, error) {
	var fp string
	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{ssh.Password(pass), ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
			ans := make([]string, len(qs))
			for i := range qs {
				ans[i] = pass
			}
			return ans, nil
		})},
		// ホスト鍵は初回接続で検証できる情報源がないため、指紋を利用者へ表示して目視確認を促す（既知の制約）
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			fp = ssh.FingerprintSHA256(key)
			return nil
		},
		Timeout: 8 * time.Second,
	}
	cl, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("SSH接続に失敗 %s: %w", addr, err)
	}
	s, err := cl.NewSession()
	if err != nil {
		cl.Close()
		return nil, "", err
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 115200, ssh.TTY_OP_OSPEED: 115200}
	if err := s.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		s.Close()
		cl.Close()
		return nil, "", err
	}
	in, err := s.StdinPipe()
	if err != nil {
		cl.Close()
		return nil, "", err
	}
	out, err := s.StdoutPipe()
	if err != nil {
		cl.Close()
		return nil, "", err
	}
	s.Stderr = io.Discard
	if err := s.Shell(); err != nil {
		cl.Close()
		return nil, "", err
	}
	return &sshSession{cl: cl, s: s, in: in, out: out}, fp, nil
}

func (s *sshSession) Read(p []byte) (int, error)  { return s.out.Read(p) }
func (s *sshSession) Write(p []byte) (int, error) { return s.in.Write(p) }
func (s *sshSession) Resize(c, r int) error       { return s.s.WindowChange(r, c) }
func (s *sshSession) Close() error {
	s.s.Close()
	return s.cl.Close()
}

// ---- Telnet（IACネゴシエーション処理） ----

const (
	iac     = 255
	dont    = 254
	do      = 253
	wont    = 252
	will    = 251
	sb      = 250
	se      = 240
	optEcho = 1
	optSGA  = 3
	optNAWS = 31
)

type telnet struct {
	c          net.Conn
	cols, rows int
	pending    []byte
	nawsOK     bool
}

// Read は受信データからIACシーケンスを取り除き、オプション要求へ応答する。
func (t *telnet) Read(p []byte) (int, error) {
	buf := make([]byte, len(p))
	for {
		n, err := t.c.Read(buf)
		if n > 0 {
			data := append(t.pending, buf[:n]...)
			t.pending = nil
			out := t.filter(data)
			if len(out) > 0 {
				return copy(p, out), nil
			}
		}
		if err != nil {
			return 0, err
		}
	}
}

func (t *telnet) filter(d []byte) []byte {
	var out bytes.Buffer
	for i := 0; i < len(d); i++ {
		b := d[i]
		if b != iac {
			out.WriteByte(b)
			continue
		}
		if i+1 >= len(d) {
			t.pending = d[i:]
			break
		}
		cmd := d[i+1]
		switch cmd {
		case iac:
			out.WriteByte(iac)
			i++
		case do, dont, will, wont:
			if i+2 >= len(d) {
				t.pending = d[i:]
				return out.Bytes()
			}
			t.negotiate(cmd, d[i+2])
			i += 2
		case sb:
			// サブネゴシエーションは SE まで読み飛ばす
			j := bytes.Index(d[i:], []byte{iac, se})
			if j < 0 {
				t.pending = d[i:]
				return out.Bytes()
			}
			i += j + 1
		default:
			i++
		}
	}
	return out.Bytes()
}

func (t *telnet) negotiate(cmd, opt byte) {
	switch cmd {
	case do:
		if opt == optNAWS {
			t.c.Write([]byte{iac, will, optNAWS})
			t.nawsOK = true
			t.sendNAWS()
		} else if opt == optSGA {
			t.c.Write([]byte{iac, will, opt})
		} else {
			t.c.Write([]byte{iac, wont, opt})
		}
	case will:
		if opt == optEcho || opt == optSGA {
			t.c.Write([]byte{iac, do, opt})
		} else {
			t.c.Write([]byte{iac, dont, opt})
		}
	}
}

func (t *telnet) sendNAWS() {
	if !t.nawsOK {
		return
	}
	t.c.Write([]byte{iac, sb, optNAWS, byte(t.cols >> 8), byte(t.cols), byte(t.rows >> 8), byte(t.rows), iac, se})
}

func (t *telnet) Write(p []byte) (int, error) {
	// 0xFF はエスケープして送る
	q := bytes.ReplaceAll(p, []byte{iac}, []byte{iac, iac})
	if _, err := t.c.Write(q); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (t *telnet) Resize(c, r int) error {
	t.cols, t.rows = c, r
	t.sendNAWS()
	return nil
}

func (t *telnet) Close() error { return t.c.Close() }

// ErrIdle はアイドルタイムアウトで切断したことを示す。
var ErrIdle = errors.New("無操作のため切断しました")
