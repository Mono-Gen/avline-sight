package sim

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/snmpx"
)

type entry struct {
	oid []uint32
	pdu gosnmp.SnmpPDU
}

type treeBuilder struct{ es []entry }

func (b *treeBuilder) add(base string, idx []uint32, t gosnmp.Asn1BER, v any) {
	o := snmpx.MustOID(base)
	o = append(append([]uint32{}, o...), idx...)
	b.es = append(b.es, entry{o, gosnmp.SnmpPDU{Name: "." + snmpx.FormatOID(o), Type: t, Value: v}})
}

// スカラー（"…​.0" 付き定数）を追加
func (b *treeBuilder) scalar(oid string, t gosnmp.Asn1BER, v any) { b.add(oid, nil, t, v) }

func i1(v int) []uint32 { return []uint32{uint32(v)} }

func inetIdx(ip string) []uint32 {
	var a, c, d, e uint32
	fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &c, &d, &e)
	return []uint32{1, 4, a, c, d, e}
}

func ipBytes(ip string) []byte {
	var a, c, d, e byte
	if ip == "" {
		return []byte{}
	}
	fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &c, &d, &e)
	return []byte{a, c, d, e}
}

func ip4(b [4]byte) string { return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3]) }

// build はスイッチ状態から現在のMIBツリー（ソート済み）を作る。呼び出し側でロック済み。
func (s *Switch) build(lab *Lab) []entry {
	b := &treeBuilder{}
	G, C64, C32, I, OS, TT, IP := gosnmp.Gauge32, gosnmp.Counter64, gosnmp.Counter32, gosnmp.Integer, gosnmp.OctetString, gosnmp.TimeTicks, gosnmp.IPAddress
	u64 := func(f float64) uint64 { return uint64(f) }
	u32 := func(f float64) uint { return uint(uint64(f) & 0xffffffff) }

	b.scalar(mib.SysDescr, OS, []byte(fmt.Sprintf("%s ProSAFE AV Line Managed Switch, %s", s.Model, s.FW)))
	b.scalar(mib.SysObjectID, gosnmp.ObjectIdentifier, ".1.3.6.1.4.1.4526.100.4.82")
	b.scalar(mib.SysUpTime, TT, s.uptimeTicks())
	b.scalar(mib.SysName, OS, []byte(s.Name))
	b.scalar(mib.AgentInventoryMachineType, OS, []byte("NETGEAR "+s.Model))
	b.scalar(mib.AgentInventoryMachineModel, OS, []byte(s.Model))
	b.scalar(mib.AgentInventorySerialNumber, OS, []byte(s.Serial))
	b.scalar(mib.AgentInventorySoftwareVersion, OS, []byte(s.FW))
	for i, n := range []string{"RFC 1213 - MIB-II", "IF-MIB", "EtherLike-MIB", "POWER-ETHERNET-MIB", "LLDP-MIB", "FASTPATH-SWITCHING-MIB", "FASTPATH-BOXSERVICES-PRIVATE-MIB", "FASTPATH-QOS-COS-MIB", "FASTPATH-POWER-ETHERNET-MIB", "FASTPATH-DHCPSERVER-PRIVATE-MIB", "FASTPATH-LOGGING-MIB", "FASTPATH-GREEN-ETHERNET-MIB", "FASTPATH-UDLD-MIB"} {
		b.add(mib.AgentSupportedMibName, i1(i+1), OS, []byte(n))
	}

	// ポート系テーブル
	nbytes := (len(s.Ports) + 7) / 8
	for _, p := range s.Ports {
		ix := i1(p.IfIndex)
		oper := 2
		if p.Up {
			oper = 1
		}
		ifType := 6
		b.add(mib.IfDescr, ix, OS, []byte(fmt.Sprintf("Unit: 1 Slot: 0 Port: %d Gigabit - Level", p.IfIndex)))
		b.add(mib.IfType, ix, I, ifType)
		b.add(mib.IfMtu, ix, I, p.MaxFrame-18)
		b.add(mib.IfPhysAddress, ix, OS, []byte{s.BaseMAC[0], s.BaseMAC[1], s.BaseMAC[2], s.BaseMAC[3], s.BaseMAC[4], byte(p.IfIndex)})
		b.add(mib.IfAdminStatus, ix, I, 1)
		b.add(mib.IfOperStatus, ix, I, oper)
		b.add(mib.IfInDiscards, ix, C32, u32(p.InDisc))
		b.add(mib.IfInErrors, ix, C32, u32(p.InErr))
		b.add(mib.IfOutDiscards, ix, C32, u32(p.OutDisc))
		b.add(mib.IfOutErrors, ix, C32, u32(p.OutErr))
		b.add(mib.IfName, ix, OS, []byte(p.Name))
		b.add(mib.IfHCInOctets, ix, C64, u64(p.InOct))
		b.add(mib.IfHCInMulticastPkts, ix, C64, u64(p.InMc))
		b.add(mib.IfHCInBroadcastPkts, ix, C64, u64(p.InBc))
		b.add(mib.IfHCOutOctets, ix, C64, u64(p.OutOct))
		b.add(mib.IfHCOutMulticastPkts, ix, C64, u64(p.OutMc))
		b.add(mib.IfHCOutBroadcastPkts, ix, C64, u64(p.OutBc))
		spd := 0
		if p.Up {
			spd = p.SpeedMbps
		}
		b.add(mib.IfHighSpeed, ix, G, uint(spd))
		alias := ""
		if p.Dev != nil && p.Dev.Kind == DevUplink {
			alias = "Uplink to " + p.Dev.Name
		}
		b.add(mib.IfAlias, ix, OS, []byte(alias))

		b.add(mib.Dot3AlignmentErrors, ix, C32, u32(p.Align))
		b.add(mib.Dot3FCSErrors, ix, C32, u32(p.FCS))
		b.add(mib.Dot3LateCollisions, ix, C32, u32(p.LateCol))
		b.add(mib.Dot3FrameTooLongs, ix, C32, u32(p.TooLong))
		b.add(mib.Dot3SymbolErrors, ix, C32, u32(p.Symbol))
		dup := 3
		if !p.Up {
			dup = 1
		}
		b.add(mib.Dot3DuplexStatus, ix, I, dup)
		b.add(mib.Dot3TxLPITransitions, ix, C32, u32(p.LPITx))
		b.add(mib.Dot3RxLPITransitions, ix, C32, u32(p.LPIRx))
		b.add(mib.Dot3PauseOperMode, ix, I, p.PauseMode)
		b.add(mib.Dot3HCInPauseFrames, ix, C64, u64(p.PauseIn))
		b.add(mib.Dot3HCOutPauseFrames, ix, C64, u64(p.PauseOut))

		var tot float64
		for q := 0; q < 8; q++ {
			tot += p.CosDrop[q]
			b.add(mib.CosQueueStatsHCDrop, []uint32{uint32(p.IfIndex), uint32(q)}, C64, u64(p.CosDrop[q]))
		}
		b.add(mib.CosQueueIntfHCTotalDrop, ix, C64, u64(tot))
		b.add(mib.CosMapIntfTrustMode, ix, I, 4)

		// agentPortConfigTable は dot1dBasePort をインデックスとする（ここでは basePort=ifIndex）
		b.add(mib.PortCfgIfIndex, ix, I, p.IfIndex)
		b.add(mib.PortCfgMaxFrameSize, ix, I, p.MaxFrame)
		b.add(mib.PortCfgBcastCtrlMode, ix, I, 2)
		b.add(mib.PortCfgMcastCtrlMode, ix, I, 2)
		b.add(mib.Dot1dBasePortIfIdx, ix, I, p.IfIndex)
		pvid := 1
		if p.Dev != nil && p.Dev.VLAN > 0 {
			pvid = p.Dev.VLAN
		}
		b.add(mib.Dot1qPvid, ix, G, uint(pvid))

		green := 0
		if p.EEE {
			green = 1
		}
		b.add(mib.GreenEeeMode, ix, I, green)
		b.add(mib.GreenEnergyDetectMode, ix, I, 0)
		b.add(mib.UdldIntfOperStatus, ix, I, p.UdldOper)
		fwd := 3
		if p.Blocking {
			fwd = 1
		} else if !p.Up {
			fwd = 4
		}
		b.add(mib.StpCstPortFwdState, ix, I, fwd)
		b.add(mib.DhcpSnoopIfTrust, ix, I, 2)
		inv := 0.0
		if p.Dev != nil && p.Dev.Kind == DevUplink {
			inv = s.DhcpInvalid
		}
		b.add(mib.DhcpSnoopInvalidServer, ix, C32, u32(inv))

		if !p.SFP {
			// PoE（グループ1, ポート=ifIndex）
			gi := []uint32{1, uint32(p.IfIndex)}
			det, mw, ma, v := 2, 0.0, 0.0, 0
			if p.Up && p.Dev != nil && p.Dev.PoEmW > 0 {
				if p.PoeFault == 0 {
					det, mw, v = 3, p.Dev.PoEmW*(0.97+0.06*s.rng.Float64()), 53
					ma = mw / 53
				} else {
					det = 4
				}
			}
			b.add(mib.PethPortDetectionStatus, gi, I, det)
			b.add(mib.PethPortClass, gi, I, 5)
			b.add(mib.PethPortPowerDeniedCnt, gi, C32, u32(p.PoeDenied))
			b.add(mib.PethPortOverLoadCnt, gi, C32, u32(p.PoeOverload))
			b.add(mib.PethPortShortCnt, gi, C32, u32(p.PoeShort))
			b.add(mib.AgentPethOutputPower, gi, G, uint(mw))
			b.add(mib.AgentPethOutputCurrent, gi, G, uint(ma))
			b.add(mib.AgentPethOutputVolts, gi, G, uint(v))
			b.add(mib.AgentPethFaultStatus, gi, I, p.PoeFault)
		} else {
			los, txf := 2, 2
			if !p.Up {
				los = 1
			}
			b.add(mib.OpticsTemp, ix, I, 385+s.rng.Intn(20))
			b.add(mib.OpticsVoltage, ix, I, 3290+s.rng.Intn(20))
			b.add(mib.OpticsCurrent, ix, I, 68+s.rng.Intn(5))
			b.add(mib.OpticsPowerOut, ix, I, int(p.TxDBm*100))
			b.add(mib.OpticsPowerIn, ix, I, int((p.RxDBm+0.05*(s.rng.Float64()-0.5))*100))
			b.add(mib.OpticsTxFault, ix, I, txf)
			b.add(mib.OpticsLos, ix, I, los)
			b.add(mib.OpticsInfoVendor, ix, OS, []byte("NETGEAR"))
			b.add(mib.OpticsInfoPartNo, ix, OS, []byte("AXM763"))
			b.add(mib.OpticsInfoSupported, ix, OS, []byte("Yes"))
		}

		// 接続機器: FDB / LLDP / ARP / DHCP Snoopingバインド
		if p.Up && p.Dev != nil {
			d := p.Dev
			macs := [][6]byte{d.MAC}
			if d.Kind == DevHub {
				for k := 1; k < 4; k++ {
					m := d.MAC
					m[5] += byte(k * 16)
					macs = append(macs, m)
				}
			}
			for _, m := range macs {
				fi := append(i1(d.VLAN), uint32(m[0]), uint32(m[1]), uint32(m[2]), uint32(m[3]), uint32(m[4]), uint32(m[5]))
				b.add(mib.Dot1qTpFdbPort, fi, I, p.IfIndex)
				b.add(mib.Dot1qTpFdbStatus, fi, I, 3)
			}
			if d.Kind != DevUplink {
				mi := []uint32{uint32(d.MAC[0]), uint32(d.MAC[1]), uint32(d.MAC[2]), uint32(d.MAC[3]), uint32(d.MAC[4]), uint32(d.MAC[5])}
				b.add(mib.DsBindingIfIndex, mi, I, p.IfIndex)
				b.add(mib.DsBindingVlan, mi, I, d.VLAN)
				b.add(mib.DsBindingIP, mi, IP, ip4(d.IP))
				ai := append([]uint32{uint32(p.IfIndex)}, inetIdx(ip4(d.IP))...)
				b.add(mib.IpNetToPhysPhys, ai, OS, d.MAC[:])
			}
			if d.LLDP {
				li := []uint32{0, uint32(p.IfIndex), 1}
				name, desc, portID := d.Name, string(d.Kind)+" device", "eth0"
				chassis := d.MAC[:]
				mgmt := ip4(d.IP)
				if d.Kind == DevUplink && p.PeerSwitch >= 0 && lab != nil {
					peer := lab.Switches[p.PeerSwitch]
					name, desc = peer.Name, peer.Model+" ProSAFE AV Line"
					portID = fmt.Sprintf("1/0/%d", p.PeerPort)
					chassis = peer.BaseMAC[:]
					mgmt = ip4(peer.MgmtIP)
				}
				b.add(mib.LldpRemChassisID, li, OS, chassis)
				b.add(mib.LldpRemPortID, li, OS, []byte(portID))
				b.add(mib.LldpRemPortDesc, li, OS, []byte(portID))
				b.add(mib.LldpRemSysName, li, OS, []byte(name))
				b.add(mib.LldpRemSysDesc, li, OS, []byte(desc))
				b.add(mib.LldpRemManAddrIfID, append(li, inetIdx(mgmt)...), I, 1)
			}
		}
	}
	b.scalar(mib.LldpLocChassisID, OS, s.BaseMAC[:])

	// VLAN と IGMP
	vlans := make([]int, 0, len(s.VLANs))
	for v := range s.VLANs {
		vlans = append(vlans, v)
	}
	sort.Ints(vlans)
	b.add(mib.SnoopingAdminMode, i1(1), I, 1)
	for _, v := range vlans {
		var members []int
		for _, p := range s.Ports {
			if p.Dev != nil && (p.Dev.VLAN == v || p.Dev.Kind == DevUplink) {
				members = append(members, p.IfIndex)
			}
		}
		b.add(mib.Dot1qVlanStaticName, i1(v), OS, []byte(s.VLANs[v]))
		b.add(mib.Dot1qVlanStaticEgress, i1(v), OS, snmpx.MakePortList(members, nbytes))
		q := s.Querier[v]
		qi := []uint32{uint32(v), 1}
		en := func(x bool) int {
			if x {
				return 1
			}
			return 0
		}
		b.add(mib.SnoopVlanAdminMode, qi, I, en(q.Snooping))
		b.add(mib.SnoopVlanFastLeave, qi, I, 0)
		b.add(mib.QuerierVlanAdminMode, qi, I, en(q.AdminOn))
		b.add(mib.QuerierVlanOperMode, qi, I, q.Oper)
		b.add(mib.QuerierVlanAddress, qi, OS, ipBytes(q.Address))
		b.add(mib.QuerierOperVersion, qi, I, q.OperVer)
		b.add(mib.QuerierLastQuerierAddress, qi, OS, ipBytes(q.LastAddr))
		b.add(mib.QuerierLastQuerierVersion, qi, I, q.LastVer)
		b.add(mib.DhcpSnoopVlanEnable, i1(v), I, 2)
	}
	// マルチキャストグループ（受信側ポートで参加）
	groupPorts := map[string]map[int]bool{}
	reporter := map[string]string{}
	for _, p := range s.Ports {
		if p.Up && p.Dev != nil {
			for _, g := range p.Dev.Groups {
				key := fmt.Sprintf("%d|%s", p.Dev.VLAN, g)
				if groupPorts[key] == nil {
					groupPorts[key] = map[int]bool{}
				}
				groupPorts[key][p.IfIndex] = true
				reporter[fmt.Sprintf("%d|%d|%s", p.IfIndex, p.Dev.VLAN, g)] = ip4(p.Dev.IP)
			}
		}
	}
	for key, r := range reporter {
		var ifx, vl int
		var g string
		fmt.Sscanf(strings.ReplaceAll(key, "|", " "), "%d %d %s", &ifx, &vl, &g)
		gi := append([]uint32{1, uint32(ifx), uint32(vl)}, inetIdx(g)[1:]...)
		b.add(mib.SSMGroupLastReporter, gi, OS, ipBytes(r))
	}
	for key, ports := range groupPorts {
		var vl int
		var g string
		fmt.Sscanf(strings.ReplaceAll(key, "|", " "), "%d %s", &vl, &g)
		var ps []int
		for p := range ports {
			ps = append(ps, p)
		}
		gip := ipBytes(g)
		mac := []uint32{0x01, 0x00, 0x5E, uint32(gip[1] & 0x7f), uint32(gip[2]), uint32(gip[3])}
		b.add(mib.Dot1qTpGroupEgressPorts, append(i1(vl), mac...), OS, snmpx.MakePortList(ps, nbytes))
	}

	// 筐体ヘルス
	cpu := s.CPU5s + 2*s.rng.Float64()
	b.scalar(mib.CpuTotal5s, G, uint(cpu))
	b.scalar(mib.CpuTotal60s, G, uint(s.CPU5s))
	b.scalar(mib.CpuTotal300s, G, uint(s.CPU5s*0.9))
	b.scalar(mib.CpuMemFreePct, G, uint(s.MemFreePct))
	for i, n := range []string{"osapiTimer", "dot1s_task", "snoopTask", "lldpTask", "poeTask"} {
		b.add(mib.CpuProcessName, i1(i+1), OS, []byte(n))
		b.add(mib.CpuProcessPct, i1(i+1), OS, []byte(fmt.Sprintf("5 Secs (%.2f%%) 60 Secs (%.2f%%) 300 Secs (%.2f%%)", cpu/float64(i+3), s.CPU5s/float64(i+3), s.CPU5s/float64(i+4))))
	}
	b.scalar(mib.MbufsFree, G, uint(float64(s.MbufTotal)-s.MbufUsed))
	b.scalar(mib.MbufsTotal, C32, uint(s.MbufTotal))
	b.scalar(mib.MbufsUsed, C32, uint(s.MbufUsed))
	for prio := 1; prio <= 3; prio++ {
		f := 0.0
		if prio == 2 {
			f = s.MbufAllocFails
		}
		b.add(mib.MbufAllocFails, i1(prio), C32, u32(f))
	}
	b.add(mib.TempSensorTemp, []uint32{1, 0}, I, int(s.TempC))
	b.add(mib.TempSensorTemp, []uint32{1, 1}, I, int(s.TempC)-4)
	tstate := 1
	switch {
	case s.TempC >= 75:
		tstate = 3
	case s.TempC >= 65:
		tstate = 2
	}
	b.add(mib.TempUnitState, i1(1), I, tstate)
	for f := 0; f < 2; f++ {
		fi := []uint32{1, uint32(f)}
		b.add(mib.FanItemState, fi, I, s.FanState)
		b.add(mib.FanSpeed, fi, OS, []byte(fmt.Sprintf("%d", 5200+s.rng.Intn(200))))
		b.add(mib.FanDutyLevel, fi, OS, []byte("35"))
	}
	b.add(mib.PowSupplyItemState, []uint32{1, 0}, I, s.PsuState)
	if s.PoeBudgetW > 0 {
		b.add(mib.PethMainPower, i1(1), G, uint(s.PoeBudgetW))
		b.add(mib.PethMainOperStatus, i1(1), I, 1)
		b.add(mib.PethMainConsumption, i1(1), G, uint(s.poeTotalW()))
		b.add(mib.PethMainUsageThreshold, i1(1), I, 90)
	}

	// STP / DHCP / ログ / その他設定
	b.scalar(mib.StpForceVersion, I, s.StpVersion)
	b.scalar(mib.StpAdminMode, I, 1)
	b.scalar(mib.StpCstBridgePriority, G, uint(s.StpPriority))
	core := s
	if lab != nil {
		core = lab.Switches[0]
		for _, x := range lab.Switches {
			if x.StpPriority < core.StpPriority {
				core = x
			}
		}
	}
	bid := func(prio int, m [6]byte) []byte {
		return []byte{byte(prio >> 8), byte(prio), m[0], m[1], m[2], m[3], m[4], m[5]}
	}
	b.add(mib.StpMstBridgeIdentifier, i1(0), OS, bid(s.StpPriority, s.BaseMAC))
	b.add(mib.StpMstDesignatedRootID, i1(0), OS, bid(core.StpPriority, core.BaseMAC))
	b.add(mib.StpMstTopoChangeCount, i1(0), C32, u32(s.TopoChanges))

	dhcp := 2
	if s.DhcpServerOn {
		dhcp = 1
		b.add(mib.DhcpPoolName, i1(1), OS, []byte("AV-POOL"))
		n := 0
		for _, p := range s.Ports {
			if p.Dev != nil && p.Dev.Kind != DevUplink && n < 40 {
				n++
				b.add(mib.DhcpLeaseIP, i1(n), IP, ip4(p.Dev.IP))
				b.add(mib.DhcpLeaseHW, i1(n), OS, p.Dev.MAC[:])
				b.add(mib.DhcpLeasePool, i1(n), OS, []byte("AV-POOL"))
			}
		}
	}
	for _, c := range s.DhcpConflicts {
		b.add(mib.DhcpConflictType, snmpx.MustOID(c), I, 2)
	}
	b.scalar(mib.DhcpServerAdminMode, I, dhcp)
	b.scalar(mib.DhcpServerDeclineRx, C32, uint(len(s.DhcpConflicts)))
	b.scalar(mib.DhcpServerNakTx, C32, 0)
	b.scalar(mib.DhcpServerDiscoverDrop, C32, 0)
	b.scalar(mib.DhcpSnoopingAdminMode, I, 2)
	for _, l := range s.Logs {
		b.add(mib.LogInMemoryMsgText, i1(l.Index), OS, []byte(l.Text))
	}
	b.scalar(mib.CableTesterIfIndex, G, uint(s.Cable.IfIndex))
	b.scalar(mib.CableTesterCableStatus, I, s.Cable.Status)
	b.scalar(mib.CableTesterMinLen, G, uint(s.Cable.MinLen))
	b.scalar(mib.CableTesterMaxLen, G, uint(s.Cable.MaxLen))
	b.scalar(mib.CableTesterFailLoc, G, uint(s.Cable.FailLoc))
	b.scalar(mib.BonjourAdminMode, I, 1)
	b.scalar(mib.SSLAdminMode, I, 1)
	b.scalar(mib.SSLSecurePort, I, 443)
	b.scalar(mib.SSHAdminMode, I, 1)
	b.scalar(mib.SSHMgmtPortNum, I, 22)
	b.scalar(mib.TelnetMgmtPortNum, I, 23)
	b.scalar(mib.MvrAdminMode, I, 2)
	b.scalar(mib.MulticastStormAction, I, 0)
	b.scalar(mib.BroadcastControlModeGlobal, I, 2)
	b.scalar(mib.MulticastControlModeGlobal, I, 2)

	sort.Slice(b.es, func(i, j int) bool { return snmpx.CompareOID(b.es[i].oid, b.es[j].oid) < 0 })
	return b.es
}
