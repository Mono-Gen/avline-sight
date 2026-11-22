package sim

import (
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"
)

// Scenario は障害注入シナリオ（仕様書7.1）。
type Scenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

var Scenarios = []Scenario{
	{"link-flap", "ポート瞬断＋1G→100M落ち＋CRC/FCS増分"},
	{"querier-hijack", "VLAN30 IGMPクエリア奪取（想定外IP＋v2/v3混在）"},
	{"mcast-flood", "Unknown Multicastフラッディング"},
	{"dhcp-rogue", "二重DHCP＋不正DHCP＋IP衝突"},
	{"sfp-degrade", "SFP光受信パワー低下＋UDLD片方向障害"},
	{"av-landmines", "EEE有効/PAUSE発生/FrameTooLongs"},
	{"poe-overload", "PoE過負荷（Overload/PowerDenied）"},
	{"chassis", "CPU高騰・Mbuf確保失敗・温度上昇・電源瞬断"},
	{"stp-loop", "STP Blocking出現＋MSTP設定＋TCN増加"},
	{"avb-mvr", "AVB/Milan帯域予約失敗＋MVR IGMP受信失敗"},
	{"clock-skew", "スイッチの時計が2分進む"},
}

func vbInt(oid string, v int) extraVB {
	return gosnmp.SnmpPDU{Name: "." + oid, Type: gosnmp.Integer, Value: v}
}

// Inject は指定スイッチへ障害を注入する。
func (l *Lab) Inject(sw int, id string) error {
	if sw < 0 || sw >= len(l.Switches) {
		return fmt.Errorf("スイッチ番号 %d は範囲外です（0〜%d）", sw, len(l.Switches)-1)
	}
	s := l.Switches[sw]
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advance(time.Now())
	firstDev := func() *Port {
		for _, p := range s.Ports {
			if p.Up && p.Dev != nil && p.Dev.Kind != DevUplink && !p.SFP {
				return p
			}
		}
		return s.Ports[0]
	}
	switch id {
	case "link-flap":
		p := firstDev()
		p.Up = false
		s.addLog("TRAPMGR: Link Down: %s", p.Name)
		s.trap("linkDown", vbInt("1.3.6.1.2.1.2.2.1.1", p.IfIndex))
		go func() {
			time.Sleep(3 * time.Second)
			s.mu.Lock()
			s.advance(time.Now())
			p.Up, p.SpeedMbps, p.FCSRate = true, 100, 4
			s.addLog("TRAPMGR: Link Up: %s (100 Mbps)", p.Name)
			s.mu.Unlock()
			s.trap("linkUp", vbInt("1.3.6.1.2.1.2.2.1.1", p.IfIndex))
		}()
	case "querier-hijack":
		q := s.Querier[30]
		if q == nil {
			return fmt.Errorf("VLAN30が存在しません")
		}
		q.LastAddr, q.LastVer, q.OperVer = "192.168.30.5", 3, 2
		if sw != 0 {
			q.Oper, q.AdminOn, q.Address = 1, true, "192.168.30.5"
		}
		s.addLog("IGMP: querier changed on VLAN 30 to 192.168.30.5 (v3)")
	case "mcast-flood":
		for _, p := range s.Ports {
			if p.Up {
				p.McastFloodPps = 30000
			}
		}
		s.addLog("SNOOP: unknown multicast flooding detected")
	case "dhcp-rogue":
		s.DhcpServerOn = true
		s.DhcpInvalid += 25
		s.DhcpConflicts = append(s.DhcpConflicts, "192.168.10.50")
		s.addLog("DHCP: address conflict detected for 192.168.10.50")
		s.trap("agentSwitchIpAddressConflictTrap")
	case "sfp-degrade":
		for _, p := range s.Ports {
			if p.SFP && p.Up {
				p.RxDBm = -12.5
				p.UdldOper = 1
				p.FCSRate = 1
				s.addLog("UDLD: unidirectional link detected on %s", p.Name)
				break
			}
		}
	case "av-landmines":
		p := firstDev()
		p.EEE, p.LPIRate = true, 15
		p.PauseMode, p.PauseRate = 4, 40
		p.TooLongRate = 2
		s.addLog("GREEN: EEE enabled on %s", p.Name)
	case "poe-overload":
		for _, p := range s.Ports {
			if p.Up && p.Dev != nil && p.Dev.PoEmW > 0 {
				p.PoeFault = 3
				p.PoeOverload++
				p.PoeDenied++
				s.addLog("POE: overload on %s", p.Name)
				s.trap("pethPsePortOnOffNotification", vbInt("1.3.6.1.2.1.105.1.1.1.6.1."+fmt.Sprint(p.IfIndex), 4))
				break
			}
		}
		s.trap("pethMainPowerUsageOnNotification")
	case "chassis":
		s.CPU5s, s.MemFreePct, s.MbufAllocFails, s.TempC = 92, 9, s.MbufAllocFails+120, 71
		s.addLog("BOX: temperature warning 71C")
		s.trap("agentSwitchCpuRisingThresholdTrap")
		s.trap("boxsTemperatureStateChange")
		go func() {
			time.Sleep(4 * time.Second)
			s.mu.Lock()
			s.bootTime = time.Now() // sysUpTime巻き戻り＝電源瞬断
			s.addLog("BOOT: unexpected restart (power interruption)")
			s.mu.Unlock()
			s.trap("coldStart")
		}()
	case "stp-loop":
		s.StpVersion = 3
		s.TopoChanges += 30
		for _, p := range s.Ports {
			if p.SFP && !p.Up {
				p.Up, p.Blocking = true, true
				p.Dev = &Device{Kind: DevUplink, Name: "loop", MAC: s.BaseMAC, VLAN: 1}
				break
			}
		}
		s.trap("stpInstanceTopologyChangeTrap")
	case "avb-mvr":
		s.AVBDemo = true
		s.MvrRxFail += 7
		s.addLog("MSRP: reservation failed for stream 0x28C68E1000000001 (insufficient bandwidth)")
	case "clock-skew":
		s.clockSkew = 2 * time.Minute
	default:
		return fmt.Errorf("未知のシナリオ %q", id)
	}
	return nil
}

// Reset は全障害を解除し初期状態へ戻す（カウンタの累積値は単調増加のため維持）。
func (l *Lab) Reset() {
	fresh := NewLab(len(l.Switches), 1)
	for i, s := range l.Switches {
		s.mu.Lock()
		f := fresh.Switches[i]
		for k, p := range s.Ports {
			fp := f.Ports[k]
			p.Up, p.SpeedMbps, p.Dev, p.Blocking = fp.Up, fp.SpeedMbps, fp.Dev, false
			p.FCSRate, p.TooLongRate, p.PauseRate, p.LPIRate, p.DiscRate, p.McastFloodPps = 0, 0, 0, 0, 0, 0
			p.EEE, p.PauseMode, p.PoeFault, p.RxDBm, p.UdldOper = false, 1, 0, fp.RxDBm, fp.UdldOper
			if fp.PeerSwitch >= 0 {
				p.PeerSwitch, p.PeerPort = fp.PeerSwitch, fp.PeerPort
			}
		}
		s.Querier, s.DhcpServerOn, s.DhcpConflicts = f.Querier, f.DhcpServerOn, nil
		s.CPU5s, s.MemFreePct, s.TempC, s.StpVersion = f.CPU5s, f.MemFreePct, f.TempC, f.StpVersion
		s.AVBDemo = false
		s.clockSkew = 0
		s.addLog("LAB: faults cleared")
		s.mu.Unlock()
	}
}
