package sim

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// startCLI は簡易モックTelnet CLIを起動する。ログイン後、主要 show コマンドに仮想応答を返す。
func (l *Lab) startCLI(ctx context.Context, s *Switch, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("仮想CLI待受に失敗 (%s): %w", addr, err)
	}
	go func() { <-ctx.Done(); ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go l.cliSession(s, c)
		}
	}()
	return nil
}

func (l *Lab) cliSession(s *Switch, c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := func(f string, a ...any) { io.WriteString(c, toCRLF(fmt.Sprintf(f, a...))) }
	readLine := func() (string, bool) {
		c.SetReadDeadline(time.Now().Add(15 * time.Minute))
		line, err := r.ReadString('\n')
		if err != nil {
			return "", false
		}
		// Telnet IAC シーケンスを除去
		var b strings.Builder
		for i := 0; i < len(line); i++ {
			if line[i] == 0xFF && i+2 < len(line) {
				i += 2
				continue
			}
			b.WriteByte(line[i])
		}
		return strings.TrimSpace(b.String()), true
	}
	l.cliLoop(s, w, readLine, true)
}

// cliLoop はTelnet/SSH共通のCLI対話ループ。login=true ならUser/Passwordプロンプトを出す
// （SSHはトランスポート層で認証済みのため省略する）。
func (l *Lab) cliLoop(s *Switch, w func(string, ...any), readLine func() (string, bool), login bool) {
	if login {
		w("\nUser:")
		if _, ok := readLine(); !ok {
			return
		}
		w("Password:")
		if _, ok := readLine(); !ok {
			return
		}
	}
	prompt := fmt.Sprintf("(%s) #", s.Name)
	w("\n%s", prompt)
	for {
		cmd, ok := readLine()
		if !ok {
			return
		}
		switch {
		case cmd == "":
		case cmd == "exit" || cmd == "quit" || cmd == "logout":
			w("\nGoodbye.\n")
			return
		case strings.HasPrefix(cmd, "show"):
			w("\n%s\n", l.showOutput(s, cmd))
		default:
			w("\n%% Lab mock CLI: '%s' は未対応です（show系コマンドのみ応答）\n", cmd)
		}
		w("%s", prompt)
	}
}

func (l *Lab) showOutput(s *Switch, cmd string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	switch {
	case strings.HasPrefix(cmd, "show igmpsnooping querier"):
		b.WriteString("VLAN  Admin  Oper         Querier Address   Last Querier      Ver\n")
		for v, q := range s.Querier {
			oper := []string{"Disabled", "Querier", "Non-Querier"}[q.Oper]
			fmt.Fprintf(&b, "%-5d %-6v %-12s %-17s %-17s %d\n", v, q.AdminOn, oper, q.Address, q.LastAddr, q.LastVer)
		}
	case strings.HasPrefix(cmd, "show igmpsnooping mrouter"):
		b.WriteString("Interface  VLAN  Type\n")
		for _, p := range s.Ports {
			if p.Dev != nil && p.Dev.Kind == DevUplink {
				fmt.Fprintf(&b, "%-10s all   dynamic\n", p.Name)
			}
		}
	case strings.HasPrefix(cmd, "show igmpsnooping"):
		b.WriteString("Admin Mode: Enable\nMulticast Control Frame Count: 18342\nVLANs enabled for IGMP snooping: 1,10,20,30,40\n")
	case strings.HasPrefix(cmd, "show poe"):
		fmt.Fprintf(&b, "Power budget: %.0f W  Consumed: %.1f W\nIntf     Status            Power(mW)\n", s.PoeBudgetW, s.poeTotalW())
		for _, p := range s.Ports {
			if !p.SFP && p.Dev != nil && p.Dev.PoEmW > 0 {
				st := "Delivering Power"
				if p.PoeFault != 0 {
					st = "Fault"
				}
				fmt.Fprintf(&b, "%-8s %-17s %.0f\n", p.Name, st, p.Dev.PoEmW)
			}
		}
	case strings.HasPrefix(cmd, "show fiber-ports"):
		b.WriteString("Port      Temp  Voltage  Current  TxPwr(dBm) RxPwr(dBm) LOS\n")
		for _, p := range s.Ports {
			if p.SFP {
				fmt.Fprintf(&b, "%-9s 39.0  3.29     6.9      %-10.2f %-10.2f %v\n", p.Name, p.TxDBm, p.RxDBm, !p.Up)
			}
		}
	case strings.HasPrefix(cmd, "show logging"):
		for _, e := range s.Logs {
			b.WriteString(e.Text + "\n")
		}
	case strings.HasPrefix(cmd, "show mac-addr-table"):
		b.WriteString("VLAN ID  MAC Address        Interface  Status\n")
		for _, p := range s.Ports {
			if p.Dev != nil && p.Up {
				m := p.Dev.MAC
				fmt.Fprintf(&b, "%-8d %02X:%02X:%02X:%02X:%02X:%02X  %-10s Learned\n", p.Dev.VLAN, m[0], m[1], m[2], m[3], m[4], m[5], p.Name)
			}
		}
	case strings.HasPrefix(cmd, "show lldp"):
		b.WriteString("Local Interface  RemoteID  System Name\n")
		for _, p := range s.Ports {
			if p.Dev != nil && p.Dev.LLDP && p.Up {
				fmt.Fprintf(&b, "%-16s %-9d %s\n", p.Name, 1, p.Dev.Name)
			}
		}
	case strings.HasPrefix(cmd, "show running-config"):
		fmt.Fprintf(&b, "!Current Configuration:\n!System Description \"%s\"\n!System Software Version \"%s\"\nhostname \"%s\"\nset igmp\nspanning-tree mode rstp\nexit\n", s.Model, s.FW, s.Name)
	default:
		b.WriteString("% Lab mock CLI: このshowコマンドは模擬応答がありません")
	}
	return b.String()
}

// toCRLF は出力の改行をCRLFへそろえる。端末はLFだけだと行頭に戻らず階段状に表示されるため。
func toCRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}
