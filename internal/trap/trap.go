// Package trap はSNMPv2c Trapを受信・デコードし、統合イベントへ変換する。
package trap

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/model"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

// Resolver は送信元（IP・ラボ用エージェントポート）からスイッチを特定する。
type Resolver func(ip string, labPort int) (id int64, name string, ok bool)

// Receiver はTrap待受。
type Receiver struct {
	mu      sync.Mutex
	tl      *gosnmp.TrapListener
	addr    string
	err     string
	ok      bool
	onEvent func(model.Event)
	resolve Resolver
}

// Status は待受状態（UI警告表示用）。
type Status struct {
	Addr  string `json:"addr"`
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

func New(onEvent func(model.Event), resolve Resolver) *Receiver {
	return &Receiver{onEvent: onEvent, resolve: resolve}
}

// Start は addr（例 "0.0.0.0:162"）で待受を開始する。失敗してもアプリは落とさず状態に記録する（仕様8.1）。
func (r *Receiver) Start(addr string) Status {
	r.Stop()
	tl := gosnmp.NewTrapListener()
	tl.Params = &gosnmp.GoSNMP{Version: gosnmp.Version2c, Logger: gosnmp.NewLogger(nil)}
	tl.OnNewTrap = r.handle
	errc := make(chan error, 1)
	go func() { errc <- tl.Listen(addr) }()
	select {
	case err := <-errc:
		msg := fmt.Sprintf("Trap待受 %s を開始できません: %v。Windows標準の「SNMP Trap」サービスが使用中、または管理者権限不足の可能性があります。設定でポート（例: 1162）を変更してください", addr, err)
		slog.Warn(msg)
		r.mu.Lock()
		r.addr, r.ok, r.err = addr, false, msg
		r.mu.Unlock()
	case <-tl.Listening():
		r.mu.Lock()
		r.tl, r.addr, r.ok, r.err = tl, addr, true, ""
		r.mu.Unlock()
		slog.Info("Trap待受を開始", "addr", addr)
	case <-time.After(3 * time.Second):
		r.mu.Lock()
		r.addr, r.ok, r.err = addr, false, "Trap待受の開始がタイムアウトしました"
		r.mu.Unlock()
	}
	return r.Status()
}

func (r *Receiver) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tl != nil {
		r.tl.Close()
		r.tl = nil
	}
}

func (r *Receiver) Status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Status{Addr: r.addr, OK: r.ok, Error: r.err}
}

func (r *Receiver) handle(p *gosnmp.SnmpPacket, addr *net.UDPAddr) {
	ev := Decode(p)
	labPort := 0
	for _, v := range p.Variables {
		if strings.TrimPrefix(v.Name, ".") == "1.3.6.1.4.1.99999.1.0" {
			labPort = int(sx.Int(v))
		}
	}
	ip := ""
	if addr != nil {
		ip = addr.IP.String()
	}
	if id, name, ok := r.resolve(ip, labPort); ok {
		ev.SwitchID, ev.SwitchName = id, name
	} else {
		ev.SwitchName = ip
		ev.Detail["unregistered_source"] = ip
	}
	r.onEvent(ev)
}

// Decode はTrapパケットをイベントへ変換する。未知Trapも生OIDとVarBindを保存して取りこぼさない。
func Decode(p *gosnmp.SnmpPacket) model.Event {
	ev := model.Event{TS: time.Now().UTC(), Category: "TRAP", Severity: model.SevInfo, Detail: map[string]any{}}
	var trapOID string
	vbs := map[string]any{}
	for _, v := range p.Variables {
		name := strings.TrimPrefix(v.Name, ".")
		switch name {
		case mib.SnmpTrapOID:
			if s, ok := v.Value.(string); ok {
				trapOID = strings.TrimPrefix(s, ".")
			}
			continue
		case "1.3.6.1.2.1.1.3.0", "1.3.6.1.4.1.99999.1.0":
			continue
		}
		var val any
		switch v.Type {
		case gosnmp.OctetString:
			val = sx.Str(v)
		case gosnmp.ObjectIdentifier, gosnmp.IPAddress:
			val = v.Value
		default:
			val = sx.Int(v)
		}
		vbs[name] = val
		// linkDown/linkUp の ifIndex
		if idx, ok := sx.Suffix(name, "1.3.6.1.2.1.2.2.1.1"); ok && len(idx) == 1 {
			ev.IfIndex = int(sx.Int(v))
		}
		if idx, ok := sx.Suffix(name, "1.3.6.1.2.1.105.1.1.1.6"); ok && len(idx) == 2 {
			ev.IfIndex = int(idx[1])
		}
	}
	ev.Detail["varbinds"] = vbs
	ev.Detail["trap_oid"] = trapOID
	if d, ok := mib.Traps[trapOID]; ok {
		ev.Code, ev.Severity, ev.Title = d.Name, d.Severity, d.Title
	} else {
		ev.Code, ev.Title = "UNKNOWN_TRAP", "未定義Trap "+trapOID
	}
	if ev.IfIndex > 0 {
		ev.Title = fmt.Sprintf("%s（ifIndex %d）", ev.Title, ev.IfIndex)
	}
	return ev
}
