package sim

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/snmpx"
)

// Community はシミュレータの読み取りコミュニティ。
const Community = "public"

// maxResponseBytes は応答の上限。実機のUDP応答サイズ制約を模擬し、超過分のVarBindは打ち切る。
const maxResponseBytes = 8192

// Config はラボ起動設定。
type Config struct {
	BaseSNMPPort int    // 仮想エージェントのUDPポート開始番号（i番目は Base+i）
	BaseCLIPort  int    // モックTelnet CLIのTCPポート開始番号
	TrapTarget   string // Trap送信先 host:port（空なら送らない）
}

// Start は全仮想スイッチのエージェントとCLIを起動する。ctx終了で停止する。
func (l *Lab) Start(ctx context.Context, cfg Config) error {
	for _, s := range l.Switches {
		addr := fmt.Sprintf("127.0.0.1:%d", cfg.BaseSNMPPort+s.Index)
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return fmt.Errorf("仮想スイッチ%sのUDP待受に失敗 (%s): %w", s.Name, addr, err)
		}
		go func() { <-ctx.Done(); pc.Close() }()
		go l.serve(pc, s)
		if cfg.BaseCLIPort > 0 {
			if err := l.startCLI(ctx, s, fmt.Sprintf("127.0.0.1:%d", cfg.BaseCLIPort+s.Index)); err != nil {
				return err
			}
		}
		if cfg.TrapTarget != "" {
			s.trapHook = l.trapSender(s, cfg.TrapTarget, cfg.BaseSNMPPort+s.Index)
		}
	}
	// 背景の揺らぎ・ログ生成
	go l.background(ctx)
	return nil
}

func (l *Lab) background(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, s := range l.Switches {
				s.mu.Lock()
				s.advance(time.Now())
				if s.rng.Intn(20) == 0 {
					s.addLog("LLDP: neighbor keepalive on 1/0/%d", 1+s.rng.Intn(len(s.Ports)))
				}
				s.mu.Unlock()
			}
		}
	}
}

func (l *Lab) serve(pc net.PacketConn, s *Switch) {
	buf := make([]byte, 65535)
	dec := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Logger: gosnmp.NewLogger(nil)}
	for {
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Debug("sim: 受信エラー", "err", err)
			continue
		}
		req, err := dec.SnmpDecodePacket(append([]byte(nil), buf[:n]...))
		if err != nil {
			slog.Debug("sim: デコード失敗", "err", err)
			continue
		}
		if req.Community != Community {
			continue // 実機同様、コミュニティ不一致は無応答
		}
		resp := l.handle(s, req)
		out, err := resp.MarshalMsg()
		if err != nil {
			slog.Warn("sim: 応答エンコード失敗", "err", err)
			continue
		}
		for len(out) > maxResponseBytes && len(resp.Variables) > 1 {
			resp.Variables = resp.Variables[:len(resp.Variables)/2]
			if out, err = resp.MarshalMsg(); err != nil {
				break
			}
		}
		if _, err := pc.WriteTo(out, addr); err != nil {
			slog.Debug("sim: 送信失敗", "err", err)
		}
	}
}

func (l *Lab) handle(s *Switch, req *gosnmp.SnmpPacket) *gosnmp.SnmpPacket {
	s.mu.Lock()
	s.advance(time.Now())
	tree := s.build(l)
	s.mu.Unlock()

	resp := &gosnmp.SnmpPacket{Version: gosnmp.Version2c, Community: req.Community, PDUType: gosnmp.GetResponse, RequestID: req.RequestID}
	find := func(o []uint32) int { // o より大きい最初の要素
		return sort.Search(len(tree), func(i int) bool { return snmpx.CompareOID(tree[i].oid, o) > 0 })
	}
	next := func(name string) gosnmp.SnmpPDU {
		o, err := snmpx.ParseOID(name)
		if err != nil {
			return gosnmp.SnmpPDU{Name: name, Type: gosnmp.EndOfMibView}
		}
		i := find(o)
		if i >= len(tree) {
			return gosnmp.SnmpPDU{Name: name, Type: gosnmp.EndOfMibView}
		}
		return tree[i].pdu
	}
	switch req.PDUType {
	case gosnmp.GetRequest:
		for _, v := range req.Variables {
			o, err := snmpx.ParseOID(v.Name)
			pdu := gosnmp.SnmpPDU{Name: v.Name, Type: gosnmp.NoSuchObject}
			if err == nil {
				i := sort.Search(len(tree), func(i int) bool { return snmpx.CompareOID(tree[i].oid, o) >= 0 })
				if i < len(tree) && snmpx.CompareOID(tree[i].oid, o) == 0 {
					pdu = tree[i].pdu
				}
			}
			resp.Variables = append(resp.Variables, pdu)
		}
	case gosnmp.GetNextRequest:
		for _, v := range req.Variables {
			resp.Variables = append(resp.Variables, next(v.Name))
		}
	case gosnmp.GetBulkRequest:
		nonRep := int(req.NonRepeaters)
		for i, v := range req.Variables {
			if i < nonRep {
				resp.Variables = append(resp.Variables, next(v.Name))
			}
		}
		reps := int(req.MaxRepetitions)
		if reps <= 0 {
			reps = 10
		}
		if reps > 100 {
			reps = 100
		}
		cur := []string{}
		for i, v := range req.Variables {
			if i >= nonRep {
				cur = append(cur, v.Name)
			}
		}
		for r := 0; r < reps && len(cur) > 0; r++ {
			end := true
			for k, name := range cur {
				pdu := next(name)
				resp.Variables = append(resp.Variables, pdu)
				if pdu.Type != gosnmp.EndOfMibView {
					end = false
					cur[k] = pdu.Name
				}
			}
			if end {
				break
			}
		}
	default:
		resp.Error = gosnmp.GenErr
	}
	return resp
}

type extraVB = gosnmp.SnmpPDU

// trapSender は v2c Trap を監視側へ送る関数を返す。
func (l *Lab) trapSender(s *Switch, target string, srcPort int) func(string, []extraVB) {
	return func(name string, vbs []extraVB) {
		oid := mib.TrapOID(name)
		if oid == "" {
			slog.Warn("sim: 未登録Trap名", "name", name)
			return
		}
		host, port, err := net.SplitHostPort(target)
		if err != nil {
			return
		}
		var p uint16
		fmt.Sscanf(port, "%d", &p)
		g := &gosnmp.GoSNMP{Target: host, Port: p, Community: Community, Version: gosnmp.Version2c, Timeout: time.Second, Retries: 0}
		if err := g.Connect(); err != nil {
			slog.Debug("sim: Trap接続失敗", "err", err)
			return
		}
		defer g.Conn.Close()
		// 送信元識別のため、エージェントのポート番号を独自VarBindで付与する（実機には無いラボ専用情報）
		vars := append([]gosnmp.SnmpPDU{{Name: "." + LabAgentPortOID, Type: gosnmp.Integer, Value: srcPort}}, vbs...)
		trap := gosnmp.SnmpTrap{Variables: append([]gosnmp.SnmpPDU{
			{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: s.uptimeTicks()},
			{Name: "." + mib.SnmpTrapOID, Type: gosnmp.ObjectIdentifier, Value: "." + oid},
		}, vars...)}
		if _, err := g.SendTrap(trap); err != nil {
			slog.Debug("sim: Trap送信失敗", "err", err)
		}
	}
}

// LabAgentPortOID はラボ専用VarBind（実験用 enterprises.99999）。Trap受信側で送信元スイッチを識別する。
const LabAgentPortOID = "1.3.6.1.4.1.99999.1.0"

func (s *Switch) trap(name string, vbs ...extraVB) {
	if s.trapHook != nil {
		go s.trapHook(name, vbs)
	}
}
