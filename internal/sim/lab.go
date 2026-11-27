package sim

import (
	"fmt"
	"math/rand"
	"time"
)

// Lab は複数台の仮想M4250群。スイッチ0をコア、残りをエッジとしスター型に接続する。
type Lab struct {
	Switches []*Switch
}

// AV機器テンプレート（帯域・PoEは現実的な目安値）
type devTpl struct {
	kind   DeviceKind
	name   string
	oui    [3]byte
	vlan   int
	poe    float64 // mW
	rx, tx float64 // bps
	mcast  float64 // pps
	groups []string
	lldp   bool
}

var tpls = []devTpl{
	{DevDante, "Dante-Stagebox", [3]byte{0x00, 0x1D, 0xC1}, 10, 0, 12e6, 12e6, 2000, []string{"239.69.1.1", "239.69.1.2"}, true},
	{DevDante, "Yamaha-Console", [3]byte{0x00, 0xA0, 0xDE}, 10, 0, 40e6, 40e6, 6000, []string{"239.69.1.1"}, true},
	{DevDante, "Shure-Mic-Rx", [3]byte{0x00, 0x0E, 0xDD}, 10, 12950, 3e6, 3e6, 500, []string{"239.69.2.1"}, true},
	{DevNDI, "NDI-Encoder", [3]byte{0x7C, 0x2E, 0x0D}, 20, 0, 5e6, 150e6, 12000, []string{"239.255.0.10"}, false},
	{DevCamera, "PTZ-Camera", [3]byte{0x00, 0x80, 0x45}, 20, 25500, 2e6, 120e6, 10000, nil, true},
	{DevSDVoE, "SDVoE-Tx", [3]byte{0x00, 0x05, 0xA6}, 30, 0, 1e6, 0, 0, []string{"239.30.0.1"}, false},
	{DevLighting, "sACN-Node", [3]byte{0x00, 0x09, 0x5B}, 40, 6500, 1e6, 1e6, 400, []string{"239.255.0.1"}, false},
	{DevControl, "Control-Processor", [3]byte{0x00, 0x10, 0x7F}, 1, 0, 0.5e6, 0.5e6, 0, nil, true},
}

// NewLab は n 台の仮想スイッチを生成する。seed固定で再現性を持たせる。
func NewLab(n int, seed int64) *Lab {
	if n < 1 {
		n = 1
	}
	lab := &Lab{}
	now := time.Now()
	for i := 0; i < n; i++ {
		spec := Models[i%len(Models)]
		if i == 0 {
			spec = Models[0]
		}
		s := &Switch{
			Index:       i,
			Name:        fmt.Sprintf("AV-SW-%02d", i+1),
			Model:       spec.Name,
			FW:          "13.0.7.6",
			Serial:      fmt.Sprintf("6LX2%07d", 1000+i),
			BaseMAC:     [6]byte{0x28, 0xC6, 0x8E, 0x10, byte(i), 0x00},
			MgmtIP:      [4]byte{192, 168, 100, byte(11 + i)},
			VLANs:       map[int]string{1: "default", 10: "Dante", 20: "Video", 30: "SDVoE", 40: "Lighting"},
			bootTime:    now.Add(-time.Duration(3600*24*(i+2)) * time.Second),
			last:        now,
			rng:         rand.New(rand.NewSource(seed + int64(i))),
			CPU5s:       8 + float64(i%3),
			MemFreePct:  58,
			MbufTotal:   4096,
			TempC:       41 + float64(i%4),
			FanState:    2,
			PsuState:    2,
			PoeBudgetW:  spec.PoEW,
			StpVersion:  2,
			StpPriority: 32768,
			Querier:     map[int]*QuerierState{},
			EEEGlobal:   false,
			Cable:       CableResult{IfIndex: 3, Status: 1, MinLen: 12, MaxLen: 15},
		}
		if i == 0 {
			s.StpPriority = 4096
			s.DhcpServerOn = true
		}
		for v := range s.VLANs {
			q := &QuerierState{Snooping: true, OperVer: 2, LastVer: 2}
			if i == 0 {
				q.AdminOn, q.Oper = true, 1
				q.Address = fmt.Sprintf("192.168.%d.1", v)
				q.LastAddr = q.Address
			} else {
				q.Oper = 2
				q.LastAddr = fmt.Sprintf("192.168.%d.1", v)
			}
			s.Querier[v] = q
		}
		total := spec.Copper + spec.SFP
		for p := 1; p <= total; p++ {
			port := &Port{IfIndex: p, Name: fmt.Sprintf("1/0/%d", p), SFP: p > spec.Copper,
				PeerSwitch: -1, PauseMode: 1, MaxFrame: 1518, UdldOper: 4}
			if port.SFP {
				port.SpeedMbps = 10000
				port.RxDBm, port.TxDBm = -3.2, -2.4
				port.UdldOper = 3
			} else {
				port.SpeedMbps = 1000
			}
			s.Ports = append(s.Ports, port)
		}
		s.addLog("BOOT: System startup complete")
		lab.Switches = append(lab.Switches, s)
	}
	// スター型アップリンク: コアのSFP k ↔ エッジiの最後のSFP
	core := lab.Switches[0]
	sfpIdx := 0
	coreSFP := []*Port{}
	for _, p := range core.Ports {
		if p.SFP {
			coreSFP = append(coreSFP, p)
		}
	}
	for i := 1; i < n && sfpIdx < len(coreSFP); i++ {
		e := lab.Switches[i]
		ep := e.Ports[len(e.Ports)-1]
		cp := coreSFP[sfpIdx]
		sfpIdx++
		link(core, cp, e, ep)
		// 最初のエッジとは2本目のSFPでもつなぎ、LAGにする（双方に空きSFPがある場合だけ）
		if i == 1 && sfpIdx < len(coreSFP) && len(e.Ports) >= 2 && e.Ports[len(e.Ports)-2].SFP {
			cp2, ep2 := coreSFP[sfpIdx], e.Ports[len(e.Ports)-2]
			sfpIdx++
			link(core, cp2, e, ep2)
			addLAG(core, cp.IfIndex, cp2.IfIndex)
			addLAG(e, ep.IfIndex, ep2.IfIndex)
		}
	}
	// 機器の割当て（各スイッチの銅線ポートのおよそ7割）
	for _, s := range lab.Switches {
		k := 0
		for _, p := range s.Ports {
			if p.SFP || p.IfIndex%10 == 0 {
				continue // 空きポートを残す
			}
			t := tpls[(k+s.Index)%len(tpls)]
			k++
			mac := [6]byte{t.oui[0], t.oui[1], t.oui[2], byte(s.Index), byte(p.IfIndex), byte(k)}
			ip := [4]byte{192, 168, byte(t.vlan), byte(20 + s.Index*40 + p.IfIndex)}
			p.Dev = &Device{Kind: t.kind, Name: fmt.Sprintf("%s-%d", t.name, k), MAC: mac, IP: ip, VLAN: t.vlan,
				LLDP: t.lldp, PoEmW: t.poe, RxBps: t.rx, TxBps: t.tx, McastTx: t.mcast, Groups: t.groups}
			p.Up = true
		}
	}
	return lab
}

func link(a *Switch, ap *Port, b *Switch, bp *Port) {
	ap.Up, bp.Up = true, true
	ap.PeerSwitch, ap.PeerPort = b.Index, bp.IfIndex
	bp.PeerSwitch, bp.PeerPort = a.Index, ap.IfIndex
	ap.Dev = &Device{Kind: DevUplink, Name: b.Name, MAC: b.BaseMAC, LLDP: true, RxBps: 300e6, TxBps: 450e6, McastTx: 20000, VLAN: 1}
	bp.Dev = &Device{Kind: DevUplink, Name: a.Name, MAC: a.BaseMAC, LLDP: true, RxBps: 450e6, TxBps: 300e6, McastTx: 20000, VLAN: 1}
}

// addLAG は物理ポートの後ろ（CPUポートの次）に LAG を1本足す。
func addLAG(s *Switch, members ...int) {
	ix := len(s.Ports) + 2 + len(s.LAGs)
	s.LAGs = append(s.LAGs, LAG{IfIndex: ix, Name: fmt.Sprintf("lag %d", len(s.LAGs)+1), Members: members})
}
