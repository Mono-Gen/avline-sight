package sim

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshHandshakeTimeout はSSHハンドシェイク（鍵交換＋認証）の上限。未完了の接続を放置しないため。
const sshHandshakeTimeout = 10 * time.Second

// sshIdleTimeout は無操作切断までの時間。Telnetモックの15分と揃える。
const sshIdleTimeout = 15 * time.Minute

// newSSHHostKey はラボ起動ごとの使い捨てホスト鍵を生成する。
// 起動のたびに指紋が変わるが、ラボは検証用なので鍵を保存しない（秘密鍵をディスクに残さないため）。
func newSSHHostKey() (ssh.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("SSHホスト鍵の生成に失敗: %w", err)
	}
	return ssh.NewSignerFromKey(priv)
}

// startSSH はモックSSH CLIを起動する。パスワード/キーボード対話認証はどの値でも受け付ける
// （127.0.0.1限定の検証用。本物の認証情報を受けても保存・ログ出力しない）。
func (l *Lab) startSSH(ctx context.Context, s *Switch, addr string, key ssh.Signer) error {
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
		KeyboardInteractiveCallback: func(ssh.ConnMetadata, ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			return nil, nil
		},
		ServerVersion: "SSH-2.0-AVLineSight-LabMock",
	}
	cfg.AddHostKey(key)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("仮想SSH待受に失敗 (%s): %w", addr, err)
	}
	go func() { <-ctx.Done(); ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go l.sshConn(s, c, cfg)
		}
	}()
	return nil
}

func (l *Lab) sshConn(s *Switch, c net.Conn, cfg *ssh.ServerConfig) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(sshHandshakeTimeout))
	sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	c.SetDeadline(time.Time{})
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "session のみ対応")
			continue
		}
		ch, creqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go l.sshSession(s, c, ch, creqs)
	}
}

func (l *Lab) sshSession(s *Switch, c net.Conn, ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	started := make(chan struct{})
	go func() {
		shell := false
		for r := range reqs {
			ok := false
			switch r.Type {
			case "pty-req", "window-change", "env":
				ok = true
			case "shell":
				ok = !shell
				if ok {
					shell = true
					close(started)
				}
			}
			if r.WantReply {
				r.Reply(ok, nil)
			}
		}
		if !shell {
			close(started)
		}
	}()
	<-started
	w := func(f string, a ...any) { io.WriteString(ch, toCRLF(fmt.Sprintf(f, a...))) }
	rd := bufio.NewReader(ch)
	readLine := func() (string, bool) {
		c.SetReadDeadline(time.Now().Add(sshIdleTimeout))
		return readEditedLine(rd, ch)
	}
	l.cliLoop(s, w, readLine, false)
	status := make([]byte, 4)
	binary.BigEndian.PutUint32(status, 0)
	ch.SendRequest("exit-status", false, status)
}

// readEditedLine はPTY上の生キー入力から1行を組み立てる。SSHのPTYではサーバー側がエコーと
// 行編集を担うため、表示可能文字のエコー・Backspace・Ctrl-C・矢印キー等のESCシーケンス破棄を行う。
func readEditedLine(r *bufio.Reader, echo io.Writer) (string, bool) {
	var b []byte
	for {
		ch, err := r.ReadByte()
		if err != nil {
			return "", false
		}
		switch {
		case ch == '\r' || ch == '\n':
			if ch == '\r' {
				// CRLF / CR NUL の後続を読み捨てる。既に届いている分だけ見る（Peekは未着だと次の入力まで待つため）
				if n, err := r.Peek(min(r.Buffered(), 1)); err == nil && len(n) == 1 && (n[0] == '\n' || n[0] == 0) {
					r.ReadByte()
				}
			}
			echo.Write([]byte("\r\n"))
			return strings.TrimSpace(string(b)), true
		case ch == 0x7f || ch == 0x08:
			if len(b) > 0 {
				b = b[:len(b)-1]
				echo.Write([]byte("\b \b"))
			}
		case ch == 0x03: // Ctrl-C: 入力行を破棄
			b = b[:0]
			echo.Write([]byte("^C\r\n"))
			return "", true
		case ch == 0x04: // Ctrl-D: 空行ならセッション終了
			if len(b) == 0 {
				return "exit", true
			}
		case ch == 0x1b: // ESC [ ... 終端英字 まで読み捨て
			// 単独のESCキーで次の入力まで待たないよう、既着分だけ見る
			if n, err := r.Peek(min(r.Buffered(), 1)); err == nil && len(n) == 1 && (n[0] == '[' || n[0] == 'O') {
				r.ReadByte()
				for {
					x, err := r.ReadByte()
					if err != nil {
						return "", false
					}
					if (x >= 'A' && x <= 'Z') || (x >= 'a' && x <= 'z') || x == '~' {
						break
					}
				}
			}
		case ch >= 0x20 && ch < 0x7f:
			if len(b) < 512 {
				b = append(b, ch)
				echo.Write([]byte{ch})
			}
		}
	}
}
