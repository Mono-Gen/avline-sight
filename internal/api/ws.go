package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/sim"
	"github.com/Mono-Gen/avline-sight/internal/term"
)

func (s *Server) isLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.locked
}

func simScenarioList() []sim.Scenario { return sim.Scenarios }

func (s *Server) accept(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	// 既定のOriginチェック（同一ホストのみ）を利用し、他サイトからのWS接続を拒否する
	return websocket.Accept(w, r, &websocket.AcceptOptions{})
}

func (s *Server) wsLive(w http.ResponseWriter, r *http.Request) {
	conn, err := s.accept(w, r)
	if err != nil {
		return
	}
	hello := encode("overview_tick", s.Engine.Overview(false))
	s.Hub.serve(s.Ctx, conn, hello)
}

// RunTicker は overview_tick を2秒毎に配信する。
func (s *Server) RunTicker(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Hub.Broadcast("overview_tick", s.Engine.Overview(false))
		}
	}
}

const terminalIdle = 10 * time.Minute

// wsTerminal は内蔵Telnet/SSHブリッジ。登録スイッチのみ・ロック中拒否・アイドル切断（仕様3.9③）。
func (s *Server) wsTerminal(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	locked := s.locked
	s.mu.Unlock()
	if locked {
		writeErr(w, 403, "閲覧専用ロック中はターミナルを使用できません", "", "ロックを解除してください")
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, 400, "不正なID", "", "")
		return
	}
	cfg, ok := s.findSwitch(id)
	if !ok {
		// ホワイトリスト: アクティブプロファイルの登録スイッチ以外へは接続しない（踏み台・SSRF防止）
		writeErr(w, 403, "接続を拒否しました", "登録されていないスイッチです", "スイッチを登録してから接続してください")
		return
	}
	proto := r.URL.Query().Get("proto")
	if proto != "ssh" {
		proto = "telnet"
	}
	conn, err := s.accept(w, r)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// 1) 認証メッセージ
	actx, acancel := context.WithTimeout(ctx, 2*time.Minute)
	_, data, err := conn.Read(actx)
	acancel()
	if err != nil {
		return
	}
	var auth struct {
		Type, User, Password string
		Cols, Rows           int
	}
	if json.Unmarshal(data, &auth) != nil || auth.Type != "auth" {
		conn.Close(websocket.StatusPolicyViolation, "最初に認証情報を送信してください")
		return
	}
	if auth.Cols <= 0 || auth.Cols > 500 {
		auth.Cols = 120
	}
	if auth.Rows <= 0 || auth.Rows > 200 {
		auth.Rows = 32
	}
	port := 23
	host := cfg.IP
	st := s.Engine.State(id)
	if proto == "ssh" {
		port = 22
		if st != nil && st.Config.SSHPort > 0 {
			port = st.Config.SSHPort
		}
	} else if st != nil && st.Config.TelnetPort > 0 {
		port = st.Config.TelnetPort
	}
	// ラボ: 仮想スイッチのモックCLIへ接続
	if s.Lab != nil && host == "127.0.0.1" && proto == "telnet" {
		port = LabCLIBase + (cfg.SNMPPort - LabSNMPBase)
	}
	// 認証待ちの間にロックされた場合は接続しない
	if s.isLocked() {
		auth.Password = ""
		conn.Close(websocket.StatusPolicyViolation, "閲覧専用ロック中のため接続できません")
		return
	}
	sess, fp, err := term.Dial(ctx, proto, host, port, auth.User, auth.Password, auth.Cols, auth.Rows)
	auth.Password = "" // メモリ上の認証情報を速やかに破棄
	if err != nil {
		slog.Info("ターミナル接続失敗", "switch", cfg.Name, "proto", proto, "err", err)
		conn.Close(websocket.StatusTryAgainLater, truncate("接続失敗: "+err.Error(), 38))
		return
	}
	defer sess.Close()
	s.Engine.OnEvent(model.Event{SwitchID: id, SwitchName: cfg.Name, Category: "SYSTEM", Severity: model.SevInfo, Code: "TERMINAL_OPEN",
		Title: "内蔵ターミナル接続（" + strings.ToUpper(proto) + "）"})
	if fp != "" {
		conn.Write(ctx, websocket.MessageText, []byte("\r\n[AVLine-Sight] SSHホスト鍵 "+fp+" — 実機の鍵と一致するか確認してください\r\n"))
	}

	idle := time.NewTimer(terminalIdle)
	defer idle.Stop()
	activity := make(chan struct{}, 1)
	// 端末→ブラウザ
	go func() {
		defer cancel()
		buf := make([]byte, 8192)
		for {
			n, err := sess.Read(buf)
			if n > 0 {
				if werr := conn.Write(ctx, websocket.MessageText, []byte(strings.ToValidUTF8(string(buf[:n]), "?"))); werr != nil {
					return
				}
			}
			if err != nil {
				conn.Close(websocket.StatusNormalClosure, "スイッチ側でセッションが終了しました")
				return
			}
		}
	}()
	// アイドル監視
	go func() {
		lockCheck := time.NewTicker(2 * time.Second)
		defer lockCheck.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-lockCheck.C:
				// セッション中にロックされたら即切断（ロック中はCLI操作を禁止する仕様）
				if s.isLocked() {
					conn.Close(websocket.StatusPolicyViolation, "閲覧専用ロックにより切断しました")
					cancel()
					return
				}
			case <-activity:
				if !idle.Stop() {
					select {
					case <-idle.C:
					default:
					}
				}
				idle.Reset(terminalIdle)
			case <-idle.C:
				conn.Close(websocket.StatusNormalClosure, "10分間操作がなかったため切断しました")
				cancel()
				return
			}
		}
	}()
	// ブラウザ→端末
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return // タブ閉鎖・切断時は defer でTCP/SSHセッションを即時終了
		}
		select {
		case activity <- struct{}{}:
		default:
		}
		var m struct {
			Type string `json:"type"`
			Data string `json:"data"`
			Cols int    `json:"cols"`
			Rows int    `json:"rows"`
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		// ロック後の入力は1文字も実機へ転送しない（定期チェックを待たずに拒否）
		if s.isLocked() {
			conn.Close(websocket.StatusPolicyViolation, "閲覧専用ロックにより切断しました")
			return
		}
		switch m.Type {
		case "data":
			if _, err := sess.Write([]byte(m.Data)); err != nil {
				return
			}
		case "resize":
			if m.Cols > 0 && m.Rows > 0 && m.Cols <= 500 && m.Rows <= 200 {
				sess.Resize(m.Cols, m.Rows)
			}
		}
	}
}

// ラボのポート割当（cmd側と共有）
const (
	LabSNMPBase = 16100
	LabCLIBase  = 2300
)

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
