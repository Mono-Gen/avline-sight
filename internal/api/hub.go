package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// Hub は /ws/live のクライアント集合。遅いクライアントは送信キュー溢れで切断し、全体を詰まらせない。
type Hub struct {
	mu      sync.Mutex
	clients map[*client]bool
}

type client struct {
	send chan []byte
	sub  int64
	mu   sync.Mutex
}

func NewHub() *Hub { return &Hub{clients: map[*client]bool{}} }

func encode(t string, data any) []byte {
	b, err := json.Marshal(map[string]any{"type": t, "data": data})
	if err != nil {
		slog.Error("WS配信データのエンコード失敗", "err", err)
		return nil
	}
	return b
}

func (h *Hub) push(c *client, b []byte) {
	select {
	case c.send <- b:
	default:
		// 送信キュー満杯: このクライアントを切り離す
		h.mu.Lock()
		if h.clients[c] {
			delete(h.clients, c)
			close(c.send)
		}
		h.mu.Unlock()
	}
}

// Broadcast は全クライアントへ配信する。
func (h *Hub) Broadcast(t string, data any) {
	b := encode(t, data)
	if b == nil {
		return
	}
	h.mu.Lock()
	cs := make([]*client, 0, len(h.clients))
	for c := range h.clients {
		cs = append(cs, c)
	}
	h.mu.Unlock()
	for _, c := range cs {
		h.push(c, b)
	}
}

// BroadcastSwitch は該当スイッチを購読中のクライアントのみへ状態を配信する。
func (h *Hub) BroadcastSwitch(id int64, st *model.SwitchState) {
	h.mu.Lock()
	var cs []*client
	for c := range h.clients {
		c.mu.Lock()
		if c.sub == id {
			cs = append(cs, c)
		}
		c.mu.Unlock()
	}
	h.mu.Unlock()
	if len(cs) == 0 {
		return
	}
	b := encode("switch_state", st)
	for _, c := range cs {
		h.push(c, b)
	}
}

// serve は1クライアントの送受信ループ。
func (h *Hub) serve(ctx context.Context, conn *websocket.Conn, hello []byte) {
	c := &client{send: make(chan []byte, 64)}
	h.mu.Lock()
	h.clients[c] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.clients[c] {
			delete(h.clients, c)
			close(c.send)
		}
		h.mu.Unlock()
		conn.CloseNow()
	}()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	// 受信: 購読要求のみ
	go func() {
		defer cancel()
		conn.SetReadLimit(4096)
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var m struct {
				Subscribe *int64 `json:"subscribe"`
			}
			if json.Unmarshal(data, &m) == nil && m.Subscribe != nil {
				c.mu.Lock()
				c.sub = *m.Subscribe
				c.mu.Unlock()
			}
		}
	}()
	if hello != nil {
		wctx, wc := context.WithTimeout(ctx, 5*time.Second)
		conn.Write(wctx, websocket.MessageText, hello)
		wc()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case b, ok := <-c.send:
			if !ok {
				conn.Close(websocket.StatusPolicyViolation, "送信が追いつかないため切断しました")
				return
			}
			wctx, wc := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, b)
			wc()
			if err != nil {
				return
			}
		}
	}
}
