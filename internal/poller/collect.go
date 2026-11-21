package poller

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/oui"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

// counters は前回値（増分計算用）。キー: "列名|ifIndex"
type counters struct {
	ts   time.Time
	vals map[string]uint64
}

func (c *counters) delta(key string, cur uint64, is32 bool) (uint64, bool) {
	prev, ok := c.vals[key]
	c.vals[key] = cur
	if !ok {
		return 0, false
	}
	return sx.CounterDelta(prev, cur, is32)
}

func ifIdx(r sx.Row) int {
	if len(r.Index) == 0 {
		return 0
	}
	return int(r.Index[0])
}

// collectSystem は system/inventory スカラーを取得する（Slow＋到達確認）。
func (w *worker) collectSystem() error {
	res, err := w.c.Get([]string{mib.SysName, mib.SysDescr, mib.SysObjectID, mib.SysUpTime,
		mib.AgentInventoryMachineModel, mib.AgentInventorySoftwareVersion, mib.AgentInventorySerialNumber, mib.LldpLocChassisID})
	if err != nil {
		return err
	}
	if _, ok := res[mib.SysUpTime]; !ok {
		return fmt.Errorf("sysUpTime が取得できません")
	}
	st := w.st
	st.SysName = sx.Str(res[mib.SysName])
	st.SysDescr = sx.Str(res[mib.SysDescr])
	if v, ok := res[mib.SysObjectID].Value.(string); ok {
		st.SysObjectID = strings.TrimPrefix(v, ".")
	}
	st.Model = sx.Str(res[mib.AgentInventoryMachineModel])
	st.FW = sx.Str(res[mib.AgentInventorySoftwareVersion])
	st.Serial = sx.Str(res[mib.AgentInventorySerialNumber])
	if b := sx.Bytes(res[mib.LldpLocChassisID]); len(b) == 6 {
		st.BaseMAC = sx.BytesToMAC(b)
	}
	ticks := int64(sx.U64(res[mib.SysUpTime]))
	up := ticks / 100
	// 再起動検知: sysUpTime の巻き戻り（仕様2.3-3）
	if w.lastUptime > 0 && ticks < w.lastUptime {
		w.emit(model.Event{Category: "ANOMALY", Severity: model.SevCritical, Code: "SWITCH_REBOOT",
			Title: "スイッチ再起動（sysUpTime巻き戻り）を検知", Hint: "電源瞬断・クラッシュ・手動再起動の可能性。電源系統と内部ログを確認してください",
			Detail: map[string]any{"prev_ticks": w.lastUptime, "cur_ticks": ticks}})
		w.cnt.vals = map[string]uint64{} // カウンタ基準をリセット
	}
	w.lastUptime = ticks
	st.UptimeSec = up
	return nil
}

// collectFast はポート状態・流量・PoE実電力（Fast Tier）。
func (w *worker) collectFast() error {
	now := time.Now()
	dt := now.Sub(w.cnt.ts).Seconds()
	first := w.cnt.ts.IsZero()
	rows, err := w.c.WalkColumns([]string{mib.IfName, mib.IfType, mib.IfOperStatus, mib.IfHighSpeed, mib.IfAlias,
		mib.IfHCInOctets, mib.IfHCOutOctets, mib.IfHCInMulticastPkts, mib.IfHCOutMulticastPkts, mib.IfHCInBroadcastPkts,
		mib.IfInErrors, mib.IfOutErrors, mib.IfInDiscards, mib.IfOutDiscards})
	if err != nil {
		return err
	}
	ports := map[int]*model.Port{}
	for _, p := range w.st.Ports {
		ports[p.IfIndex] = p
	}
	var list []*model.Port
	for _, r := range rows {
		ix := ifIdx(r)
		name := sx.Str(r.Cols[mib.IfName])
		ifType := sx.Int(r.Cols[mib.IfType])
		// 物理ポート(6=ethernetCsmacd)とLAG(161=ieee8023adLag)のみ対象。VLAN IF・CPU等は除外
		if ifType != 6 && ifType != 161 {
			continue
		}
		p := ports[ix]
		if p == nil {
			p = &model.Port{IfIndex: ix}
		}
		p.Name = name
		p.Alias = sx.Str(r.Cols[mib.IfAlias])
		if ifType == 161 || strings.HasPrefix(strings.ToLower(name), "lag") {
			p.Type = "lag"
		} else if p.Type == "" || p.Type == "lag" {
			p.Type = "rj45" // SFP判定はMedium（DOM応答有無）で上書き
		}
		oper := mib.IfOperStatusEnum.Name(int(sx.Int(r.Cols[mib.IfOperStatus])))
		spd := int(sx.Int(r.Cols[mib.IfHighSpeed]))
		if !first && p.Oper != "" && (p.Oper != oper || (oper == "up" && p.SpeedMbps != spd && p.SpeedMbps != 0)) {
			w.onLinkChange(p, oper, spd)
		}
		p.Oper, p.SpeedMbps = oper, spd
		rate := func(col string, is32 bool) (float64, uint64) {
			d, ok := w.cnt.delta(col+"|"+strconv.Itoa(ix), sx.U64(r.Cols[col]), is32)
			if !ok || dt <= 0 {
				return 0, 0
			}
			return float64(d) / dt, d
		}
		rx, _ := rate(mib.IfHCInOctets, false)
		tx, _ := rate(mib.IfHCOutOctets, false)
		p.RxBps, p.TxBps = rx*8, tx*8
		p.RxMcPps, _ = rate(mib.IfHCInMulticastPkts, false)
		p.TxMcPps, _ = rate(mib.IfHCOutMulticastPkts, false)
		p.RxBcPps, _ = rate(mib.IfHCInBroadcastPkts, false)
		_, ie := rate(mib.IfInErrors, true)
		_, oe := rate(mib.IfOutErrors, true)
		_, id := rate(mib.IfInDiscards, true)
		_, od := rate(mib.IfOutDiscards, true)
		p.ErrDelta, p.DiscardDelta = ie+oe, id+od
		p.ErrSinceReset += p.ErrDelta
		p.DiscardSinceReset += p.DiscardDelta
		if spd > 0 {
			p.UtilRx = p.RxBps / (float64(spd) * 1e6) * 100
			p.UtilTx = p.TxBps / (float64(spd) * 1e6) * 100
		} else {
			p.UtilRx, p.UtilTx = 0, 0
		}
		list = append(list, p)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].IfIndex < list[j].IfIndex })
	w.st.Ports = list
	w.cnt.ts = now

	// PoE（ポートINDEX=group,port。M4250はport番号=ifIndexの単一グループ）
	if w.supported("poe") {
		prow, err := w.c.WalkColumns([]string{mib.PethPortDetectionStatus, mib.PethPortClass, mib.PethPortPowerDeniedCnt,
			mib.PethPortOverLoadCnt, mib.PethPortShortCnt, mib.AgentPethOutputPower, mib.AgentPethOutputCurrent, mib.AgentPethOutputVolts, mib.AgentPethFaultStatus})
		w.markCap("poe", err, len(prow))
		byIdx := w.portMap()
		for _, r := range prow {
			if len(r.Index) < 2 {
				continue
			}
			p := byIdx[int(r.Index[1])]
			if p == nil {
				continue
			}
			prev := p.PoE
			poe := &model.PoE{
				Detect:      mib.PethDetectEnum.Name(int(sx.Int(r.Cols[mib.PethPortDetectionStatus]))),
				Class:       int(sx.Int(r.Cols[mib.PethPortClass])) - 1,
				PowerW:      float64(sx.U64(r.Cols[mib.AgentPethOutputPower])) / 1000, // mW→W
				CurrentMA:   int(sx.U64(r.Cols[mib.AgentPethOutputCurrent])),
				Volts:       int(sx.U64(r.Cols[mib.AgentPethOutputVolts])),
				Fault:       mib.PethFaultEnum.Name(int(sx.Int(r.Cols[mib.AgentPethFaultStatus]))),
				DeniedCnt:   sx.U64(r.Cols[mib.PethPortPowerDeniedCnt]),
				OverloadCnt: sx.U64(r.Cols[mib.PethPortOverLoadCnt]),
				ShortCnt:    sx.U64(r.Cols[mib.PethPortShortCnt]),
			}
			if prev != nil {
				w.onPoEChange(p, prev, poe)
			}
			p.PoE = poe
		}
	}
	return nil
}

func (w *worker) portMap() map[int]*model.Port {
	m := make(map[int]*model.Port, len(w.st.Ports))
	for _, p := range w.st.Ports {
		m[p.IfIndex] = p
	}
	return m
}

var reFloat = regexp.MustCompile(`-?\d+(\.\d+)?`)

func firstFloat(s string) float64 {
	m := reFloat.FindString(s)
	f, _ := strconv.ParseFloat(m, 64)
	return f
}

// collectMedium は物理層エラー・CoS・IGMP・筐体ヘルス・SFP DOM（Medium Tier）。
func (w *worker) collectMedium() error {
	pm := w.portMap()
	if w.supported("etherlike") {
		rows, err := w.c.WalkColumns([]string{mib.Dot3FCSErrors, mib.Dot3AlignmentErrors, mib.Dot3SymbolErrors, mib.Dot3FrameTooLongs,
			mib.Dot3LateCollisions, mib.Dot3DuplexStatus, mib.Dot3TxLPITransitions, mib.Dot3RxLPITransitions,
			mib.Dot3PauseOperMode, mib.Dot3HCInPauseFrames, mib.Dot3HCOutPauseFrames})
		if err != nil {
			return err
		}
		w.markCap("etherlike", nil, len(rows))
		for _, r := range rows {
			p := pm[ifIdx(r)]
			if p == nil {
				continue
			}
			k := strconv.Itoa(p.IfIndex)
			d := func(col string, is32 bool) uint64 { v, _ := w.cnt.delta("m"+col+"|"+k, sx.U64(r.Cols[col]), is32); return v }
			p.FCSDelta = d(mib.Dot3FCSErrors, true)
			p.AlignDelta = d(mib.Dot3AlignmentErrors, true)
			p.SymbolDelta = d(mib.Dot3SymbolErrors, true)
			p.TooLongDelta = d(mib.Dot3FrameTooLongs, true)
			p.LateColDelta = d(mib.Dot3LateCollisions, true)
			p.LPIDelta = d(mib.Dot3TxLPITransitions, true) + d(mib.Dot3RxLPITransitions, true)
			p.PauseDelta = d(mib.Dot3HCInPauseFrames, false) + d(mib.Dot3HCOutPauseFrames, false)
			p.Duplex = mib.DuplexEnum.Name(int(sx.Int(r.Cols[mib.Dot3DuplexStatus])))
			if v, ok := r.Cols[mib.Dot3PauseOperMode]; ok {
				p.Pause = mib.PauseModeEnum.Name(int(sx.Int(v)))
			}
		}
	}
	if w.supported("cos") {
		rows, err := w.c.WalkColumns([]string{mib.CosQueueIntfHCTotalDrop})
		w.markCap("cos", err, len(rows))
		for _, r := range rows {
			if p := pm[ifIdx(r)]; p != nil {
				p.CosDropDelta, _ = w.cnt.delta("cos|"+strconv.Itoa(p.IfIndex), sx.U64(r.Cols[mib.CosQueueIntfHCTotalDrop]), false)
			}
		}
	}
	if err := w.collectHealth(); err != nil {
		return err
	}
	if w.supported("sfp") {
		rows, err := w.c.WalkColumns([]string{mib.OpticsPowerIn, mib.OpticsPowerOut, mib.OpticsTemp, mib.OpticsVoltage, mib.OpticsCurrent, mib.OpticsLos, mib.OpticsTxFault})
		w.markCap("sfp", err, len(rows))
		for _, r := range rows {
			p := pm[ifIdx(r)]
			if p == nil {
				continue
			}
			s := p.SFP
			if s == nil {
				s = &model.SFP{}
			}
			// DBmHundreths: ÷100（仕様9.1。実機プローブで妥当範囲 -40〜+10dBm を検証）
			s.RxDBm = float64(sx.Int(r.Cols[mib.OpticsPowerIn])) / 100
			s.TxDBm = float64(sx.Int(r.Cols[mib.OpticsPowerOut])) / 100
			s.TempC = float64(sx.Int(r.Cols[mib.OpticsTemp])) / 10
			s.Volts = float64(sx.Int(r.Cols[mib.OpticsVoltage])) / 1000
			s.BiasMA = float64(sx.Int(r.Cols[mib.OpticsCurrent])) / 10
			s.LOS = mib.Truth(int(sx.Int(r.Cols[mib.OpticsLos])))
			s.TxFault = mib.Truth(int(sx.Int(r.Cols[mib.OpticsTxFault])))
			p.SFP = s
			if p.Type != "lag" {
				p.Type = "sfp"
			}
		}
	}
	return w.collectIGMP()
}

func (w *worker) collectHealth() error {
	h := &w.st.Health
	res, err := w.c.Get([]string{mib.CpuTotal5s, mib.CpuTotal60s, mib.CpuTotal300s, mib.CpuMemFreePct, mib.MbufsFree, mib.MbufsTotal, mib.MbufsUsed})
	if err != nil {
		return err
	}
	h.CPU5s = float64(sx.U64(res[mib.CpuTotal5s]))
	h.CPU60s = float64(sx.U64(res[mib.CpuTotal60s]))
	h.CPU300s = float64(sx.U64(res[mib.CpuTotal300s]))
	h.MemFreePct = float64(sx.U64(res[mib.CpuMemFreePct]))
	if tot := sx.U64(res[mib.MbufsTotal]); tot > 0 {
		h.MbufUsedPct = float64(sx.U64(res[mib.MbufsUsed])) / float64(tot) * 100
	}
	// Mbuf確保失敗は優先度別テーブルの合算（仕様9.1）
	if rows, err := w.c.WalkColumns([]string{mib.MbufAllocFails}); err == nil {
		var sum uint64
		for _, r := range rows {
			sum += sx.U64(r.Cols[mib.MbufAllocFails])
		}
		h.MbufFailDelta, _ = w.cnt.delta("mbuffail", sum, false)
		h.MbufAllocFails = sum
	}
	if w.supported("box") {
		rows, err := w.c.WalkColumns([]string{mib.TempSensorTemp})
		w.markCap("box", err, len(rows))
		h.Temps = h.Temps[:0]
		h.TempMaxC = 0
		for _, r := range rows {
			t := float64(sx.Int(r.Cols[mib.TempSensorTemp]))
			h.Temps = append(h.Temps, t)
			if t > h.TempMaxC {
				h.TempMaxC = t
			}
		}
		if rows, err := w.c.WalkColumns([]string{mib.TempUnitState}); err == nil && len(rows) > 0 {
			h.TempState = mib.BoxTempStatus.Name(int(sx.Int(rows[0].Cols[mib.TempUnitState])))
		}
		fans, _ := w.c.WalkColumns([]string{mib.FanItemState, mib.FanSpeed, mib.FanDutyLevel})
		h.Fans = h.Fans[:0]
		for i, r := range fans {
			h.Fans = append(h.Fans, model.Item{Index: i + 1, State: mib.BoxItemState.Name(int(sx.Int(r.Cols[mib.FanItemState]))),
				Speed: sx.Str(r.Cols[mib.FanSpeed]), Duty: sx.Str(r.Cols[mib.FanDutyLevel])})
		}
		psus, _ := w.c.WalkColumns([]string{mib.PowSupplyItemState})
		h.PSUs = h.PSUs[:0]
		for i, r := range psus {
			h.PSUs = append(h.PSUs, model.Item{Index: i + 1, State: mib.BoxItemState.Name(int(sx.Int(r.Cols[mib.PowSupplyItemState])))})
		}
	}
	if w.supported("poe") {
		rows, _ := w.c.WalkColumns([]string{mib.PethMainPower, mib.PethMainConsumption})
		h.PoEBudgetW, h.PoEUsedW = 0, 0
		for _, r := range rows {
			h.PoEBudgetW += float64(sx.U64(r.Cols[mib.PethMainPower]))
			h.PoEUsedW += float64(sx.U64(r.Cols[mib.PethMainConsumption]))
		}
	}
	return nil
}

// AVプロトコル推定（グループアドレスの慣用レンジ）。断定ではなくUI上のヒント。
func guessProtocol(g string) string {
	switch {
	case strings.HasPrefix(g, "239.69."):
		return "AES67/Dante"
	case strings.HasPrefix(g, "239.254."), strings.HasPrefix(g, "239.255.254."):
		return "Dante"
	case strings.HasPrefix(g, "239.255.0."), strings.HasPrefix(g, "239.255.1."):
		return "sACN"
	case g == "224.0.1.129" || g == "224.0.1.130" || g == "224.0.1.131" || g == "224.0.1.132":
		return "PTP"
	case g == "224.0.0.251":
		return "mDNS"
	case g == "239.255.255.250":
		return "SSDP"
	case strings.HasPrefix(g, "239.30."):
		return "SDVoE"
	case strings.HasPrefix(g, "239.255."):
		return "NDI/Video"
	}
	return "Unknown"
}

func (w *worker) collectIGMP() error {
	ig := &w.st.IGMP
	pm := w.portMap()
	pname := func(ix int) string {
		if p := pm[ix]; p != nil {
			return p.Name
		}
		return strconv.Itoa(ix)
	}
	res, _ := w.c.Get([]string{mib.SnoopingAdminMode + ".1"})
	ig.SnoopingOn = sx.Int(res[mib.SnoopingAdminMode+".1"]) == 1
	vrows, err := w.c.WalkColumns([]string{mib.SnoopVlanAdminMode, mib.SnoopVlanFastLeave})
	if err != nil {
		return err
	}
	qrows, _ := w.c.WalkColumns([]string{mib.QuerierVlanAdminMode, mib.QuerierVlanOperMode, mib.QuerierVlanAddress, mib.QuerierOperVersion, mib.QuerierLastQuerierAddress, mib.QuerierLastQuerierVersion})
	byV := map[int]*model.VlanIGMP{}
	var order []int
	get := func(v int) *model.VlanIGMP {
		if byV[v] == nil {
			byV[v] = &model.VlanIGMP{VLAN: v, Name: w.vlanNames[v]}
			order = append(order, v)
		}
		return byV[v]
	}
	for _, r := range vrows {
		if len(r.Index) == 2 && r.Index[1] == 1 { // IPv4のみ
			v := get(int(r.Index[0]))
			v.Snooping = sx.Int(r.Cols[mib.SnoopVlanAdminMode]) == 1
			v.FastLeave = sx.Int(r.Cols[mib.SnoopVlanFastLeave]) == 1
		}
	}
	for _, r := range qrows {
		if len(r.Index) == 2 && r.Index[1] == 1 {
			v := get(int(r.Index[0]))
			v.QuerierAdmin = sx.Int(r.Cols[mib.QuerierVlanAdminMode]) == 1
			v.QuerierOper = mib.QuerierOperEnum.Name(int(sx.Int(r.Cols[mib.QuerierVlanOperMode])))
			v.QuerierAddr = sx.BytesToIP(sx.Bytes(r.Cols[mib.QuerierVlanAddress]))
			v.LastQuerier = sx.BytesToIP(sx.Bytes(r.Cols[mib.QuerierLastQuerierAddress]))
			v.LastVersion = int(sx.Int(r.Cols[mib.QuerierLastQuerierVersion]))
			v.OperVersion = int(sx.Int(r.Cols[mib.QuerierOperVersion]))
		}
	}
	sort.Ints(order)
	ig.Vlans = ig.Vlans[:0]
	for _, v := range order {
		ig.Vlans = append(ig.Vlans, *byV[v])
	}

	// グループ: SSMGroupテーブル（ポート単位）を優先し、空ならL2 dot1qTpGroupTableへフォールバック
	ig.Groups = ig.Groups[:0]
	srows, _ := w.c.WalkColumns([]string{mib.SSMGroupLastReporter})
	type gk struct {
		vlan int
		g    string
	}
	agg := map[gk]*model.McastGroup{}
	var gorder []gk
	for _, r := range srows {
		// INDEX = atype, ifIndex, vlan, len, addr...
		if len(r.Index) < 4 {
			continue
		}
		ifx, vlan := int(r.Index[1]), int(r.Index[2])
		addr, n := sx.DecodeInetIndex(append([]uint32{r.Index[0]}, r.Index[3:]...))
		if n == 0 {
			continue
		}
		k := gk{vlan, addr}
		if agg[k] == nil {
			agg[k] = &model.McastGroup{VLAN: vlan, Group: addr, Protocol: guessProtocol(addr), Source: "ssm"}
			gorder = append(gorder, k)
		}
		agg[k].Ports = append(agg[k].Ports, pname(ifx))
		agg[k].Reporter = sx.BytesToIP(sx.Bytes(r.Cols[mib.SSMGroupLastReporter]))
	}
	if len(agg) == 0 {
		grows, _ := w.c.WalkColumns([]string{mib.Dot1qTpGroupEgressPorts})
		if len(grows) > 0 {
			w.st.Caps["igmp_groups"] = "FALLBACK_USED"
		}
		for _, r := range grows {
			if len(r.Index) < 7 {
				continue
			}
			mac := sx.IndexMAC(r.Index[1:7])
			k := gk{int(r.Index[0]), mac}
			mg := &model.McastGroup{VLAN: k.vlan, Group: mac, Protocol: "L2", Source: "l2"}
			for _, bp := range sx.PortListBits(sx.Bytes(r.Cols[mib.Dot1qTpGroupEgressPorts])) {
				mg.Ports = append(mg.Ports, pname(w.basePortToIf(bp)))
			}
			agg[k] = mg
			gorder = append(gorder, k)
		}
	} else {
		w.st.Caps["igmp_groups"] = "SUPPORTED"
	}
	for _, k := range gorder {
		ig.Groups = append(ig.Groups, *agg[k])
	}
	mrows, _ := w.c.WalkColumns([]string{mib.VlanStaticMrouterAdminMode})
	ig.Mrouters = ig.Mrouters[:0]
	for _, r := range mrows {
		if len(r.Index) >= 3 && sx.Int(r.Cols[mib.VlanStaticMrouterAdminMode]) == 1 {
			ig.Mrouters = append(ig.Mrouters, model.Mrouter{Port: pname(int(r.Index[0])), VLAN: int(r.Index[1])})
		}
	}
	// MVR / gPTP（未対応時はスキップ）
	if w.supported("avb") {
		res, err := w.c.Get([]string{mib.MvrAdminMode, mib.AsDefaultClockIdentity, mib.AsGrandmasterIdentity, mib.AsStepsRemoved})
		if err == nil {
			ig.MVR = mib.Truth(int(sx.Int(res[mib.MvrAdminMode])))
			if v, ok := res[mib.AsDefaultClockIdentity]; ok {
				ig.GPTP = &model.GPTP{ClockID: fmt.Sprintf("%X", sx.Bytes(v)), Grandmaster: fmt.Sprintf("%X", sx.Bytes(res[mib.AsGrandmasterIdentity])), StepsRemoved: int(sx.Int(res[mib.AsStepsRemoved]))}
				w.st.Caps["avb"] = "SUPPORTED"
			} else {
				ig.GPTP = nil
				w.st.Caps["avb"] = "UNSUPPORTED"
			}
		}
	}
	return nil
}

func (w *worker) basePortToIf(bp int) int {
	if ix, ok := w.basePort[bp]; ok {
		return ix
	}
	return bp
}

// collectSlow は接続機器・DHCP・ログ・設定（Slow Tier）。
func (w *worker) collectSlow() error {
	if err := w.collectSystem(); err != nil {
		return err
	}
	pm := w.portMap()
	// Bridge Port→ifIndex 変換表（仕様4.2: 必ず経由）
	if rows, err := w.c.WalkColumns([]string{mib.Dot1dBasePortIfIdx}); err == nil {
		w.basePort = map[int]int{}
		for _, r := range rows {
			w.basePort[ifIdx(r)] = int(sx.Int(r.Cols[mib.Dot1dBasePortIfIdx]))
		}
	}
	if rows, err := w.c.WalkColumns([]string{mib.Dot1qVlanStaticName}); err == nil {
		for _, r := range rows {
			w.vlanNames[ifIdx(r)] = sx.Str(r.Cols[mib.Dot1qVlanStaticName])
		}
	}
	if rows, err := w.c.WalkColumns([]string{mib.Dot1qPvid}); err == nil {
		for _, r := range rows {
			if p := pm[w.basePortToIf(ifIdx(r))]; p != nil {
				p.PVID = int(sx.Int(r.Cols[mib.Dot1qPvid]))
			}
		}
	}

	// --- 接続機器の統合（LLDP → DHCP Snooping → ARP → FDB+OUI） ---
	devs := map[int]map[string]*model.Device{} // ifIndex → MAC → 機器
	add := func(ix int, mac string) *model.Device {
		if devs[ix] == nil {
			devs[ix] = map[string]*model.Device{}
		}
		d := devs[ix][mac]
		if d == nil {
			d = &model.Device{MAC: mac, Vendor: oui.Lookup(mac), Source: "fdb"}
			devs[ix][mac] = d
		}
		return d
	}
	ipByMAC := map[string]string{}
	// FDB
	if rows, err := w.c.WalkColumns([]string{mib.Dot1qTpFdbPort, mib.Dot1qTpFdbStatus}); err == nil {
		for _, r := range rows {
			if len(r.Index) < 7 || sx.Int(r.Cols[mib.Dot1qTpFdbStatus]) != 3 { // learnedのみ
				continue
			}
			ix := w.basePortToIf(int(sx.Int(r.Cols[mib.Dot1qTpFdbPort])))
			if p := pm[ix]; p == nil || p.Type == "lag" {
				continue
			}
			d := add(ix, sx.IndexMAC(r.Index[1:7]))
			d.VLAN = int(r.Index[0])
		}
	}
	// ARP（ipNetToPhysical → フォールバック ipNetToMedia）
	arows, _ := w.c.WalkColumns([]string{mib.IpNetToPhysPhys})
	for _, r := range arows {
		if ip, n := sx.DecodeInetIndex(r.Index[1:]); n > 0 {
			ipByMAC[sx.BytesToMAC(sx.Bytes(r.Cols[mib.IpNetToPhysPhys]))] = ip
		}
	}
	if len(arows) == 0 {
		mrows, _ := w.c.WalkColumns([]string{mib.IpNetToMediaPhys})
		for _, r := range mrows {
			if len(r.Index) >= 5 {
				ipByMAC[sx.BytesToMAC(sx.Bytes(r.Cols[mib.IpNetToMediaPhys]))] = sx.IndexIPv4(r.Index[1:5])
			}
		}
	}
	// DHCP Snooping バインディング（INDEX=MAC）
	if rows, err := w.c.WalkColumns([]string{mib.DsBindingIfIndex, mib.DsBindingVlan, mib.DsBindingIP}); err == nil {
		for _, r := range rows {
			mac := sx.IndexMAC(r.Index)
			ix := int(sx.Int(r.Cols[mib.DsBindingIfIndex]))
			if pm[ix] == nil {
				continue
			}
			d := add(ix, mac)
			d.VLAN = int(sx.Int(r.Cols[mib.DsBindingVlan]))
			if ip, ok := r.Cols[mib.DsBindingIP].Value.(string); ok {
				d.IP = ip
				d.Source = "dhcp-snoop"
			}
		}
	}
	// LLDP
	w.st.Neighbors = w.st.Neighbors[:0]
	lrows, err := w.c.WalkColumns([]string{mib.LldpRemChassisID, mib.LldpRemPortID, mib.LldpRemSysName, mib.LldpRemSysDesc})
	w.markCap("lldp", err, len(lrows))
	mgmt := map[string]string{} // "localPort.remIndex" → IP
	if mrows, err := w.c.WalkColumns([]string{mib.LldpRemManAddrIfID}); err == nil {
		for _, r := range mrows {
			if len(r.Index) >= 5 {
				if ip, n := sx.DecodeInetIndex(r.Index[3:]); n > 0 && strings.Count(ip, ".") == 3 {
					mgmt[fmt.Sprintf("%d.%d", r.Index[1], r.Index[2])] = ip
				}
			}
		}
	}
	for _, r := range lrows {
		if len(r.Index) < 3 {
			continue
		}
		local := int(r.Index[1])
		ch := sx.Bytes(r.Cols[mib.LldpRemChassisID])
		chs := sx.BytesToMAC(ch)
		if chs == "" {
			chs = sx.Str(r.Cols[mib.LldpRemChassisID])
		}
		n := model.Neighbor{LocalIfIndex: local, ChassisID: chs, SysName: sx.Str(r.Cols[mib.LldpRemSysName]),
			SysDesc: sx.Str(r.Cols[mib.LldpRemSysDesc]), PortID: sx.Str(r.Cols[mib.LldpRemPortID]),
			MgmtIP: mgmt[fmt.Sprintf("%d.%d", r.Index[1], r.Index[2])]}
		n.IsNetgear = strings.Contains(strings.ToUpper(n.SysDesc), "NETGEAR") || strings.Contains(n.SysDesc, "M4250") || strings.Contains(n.SysDesc, "AV Line") || oui.Lookup(chs) == "Netgear"
		if p := pm[local]; p != nil {
			n.LocalPort = p.Name
			if chs != "" {
				d := add(local, chs)
				d.LLDPName, d.LLDPDesc, d.LLDPPort, d.Source = n.SysName, n.SysDesc, n.PortID, "lldp"
				if n.MgmtIP != "" {
					d.IP = n.MgmtIP
				}
			}
		}
		w.st.Neighbors = append(w.st.Neighbors, n)
	}
	// 統合結果をポートへ反映し、接続変化イベントを生成
	for _, p := range w.st.Ports {
		var list []model.Device
		for mac, d := range devs[p.IfIndex] {
			if d.IP == "" {
				d.IP = ipByMAC[mac]
				if d.IP != "" && d.Source == "fdb" {
					d.Source = "arp"
				}
			}
			d.LinkLocal = strings.HasPrefix(d.IP, "169.254.")
			d.Label = w.aliasFor(p.IfIndex, mac)
			list = append(list, *d)
		}
		sort.Slice(list, func(i, j int) bool { return list[i].MAC < list[j].MAC })
		w.onDevicesChange(p, list)
		p.Devices = list
	}
	w.collectDHCP(pm, ipByMAC)
	w.collectConfig(pm)
	w.collectLogs()
	return nil
}

func (w *worker) collectDHCP(pm map[int]*model.Port, _ map[string]string) {
	d := &w.st.DHCP
	res, err := w.c.Get([]string{mib.DhcpServerAdminMode, mib.DhcpServerDeclineRx, mib.DhcpSnoopingAdminMode})
	if err != nil {
		return
	}
	d.ServerOn = sx.Int(res[mib.DhcpServerAdminMode]) == 1
	d.DeclineRx = sx.U64(res[mib.DhcpServerDeclineRx])
	d.SnoopingOn = mib.Truth(int(sx.Int(res[mib.DhcpSnoopingAdminMode])))
	d.Pools, d.Leases, d.Conflicts = d.Pools[:0], d.Leases[:0], d.Conflicts[:0]
	if d.ServerOn {
		if rows, err := w.c.WalkColumns([]string{mib.DhcpPoolName}); err == nil {
			for _, r := range rows {
				d.Pools = append(d.Pools, sx.Str(r.Cols[mib.DhcpPoolName]))
			}
		}
		macPort := map[string]string{}
		for _, p := range w.st.Ports {
			for _, dv := range p.Devices {
				macPort[dv.MAC] = p.Name
			}
		}
		if rows, err := w.c.WalkColumns([]string{mib.DhcpLeaseIP, mib.DhcpLeaseHW, mib.DhcpLeasePool}); err == nil {
			for _, r := range rows {
				ip, _ := r.Cols[mib.DhcpLeaseIP].Value.(string)
				mac := sx.BytesToMAC(sx.Bytes(r.Cols[mib.DhcpLeaseHW]))
				d.Leases = append(d.Leases, model.Lease{IP: ip, MAC: mac, Pool: sx.Str(r.Cols[mib.DhcpLeasePool]), Port: macPort[mac]})
			}
		}
	}
	if rows, err := w.c.WalkColumns([]string{mib.DhcpConflictType}); err == nil {
		for _, r := range rows {
			d.Conflicts = append(d.Conflicts, sx.IndexIPv4(r.Index))
		}
	}
	if rows, err := w.c.WalkColumns([]string{mib.DhcpSnoopInvalidServer}); err == nil {
		var sum uint64
		for _, r := range rows {
			sum += sx.U64(r.Cols[mib.DhcpSnoopInvalidServer])
		}
		d.InvalidDelta, _ = w.cnt.delta("dhcpinv", sum, true)
		d.InvalidServerMsgs = sum
	}
}

func (w *worker) collectConfig(pm map[int]*model.Port) {
	cf := &w.st.Config
	res, err := w.c.Get([]string{mib.StpForceVersion, mib.StpCstBridgePriority, mib.StpMstBridgeIdentifier + ".0", mib.StpMstDesignatedRootID + ".0",
		mib.StpMstTopoChangeCount + ".0", mib.BonjourAdminMode, mib.BroadcastControlModeGlobal, mib.MulticastControlModeGlobal,
		mib.SSLAdminMode, mib.SSLSecurePort, mib.SSHAdminMode, mib.SSHMgmtPortNum, mib.TelnetMgmtPortNum,
		mib.CableTesterIfIndex, mib.CableTesterCableStatus, mib.CableTesterMinLen, mib.CableTesterMaxLen, mib.CableTesterFailLoc})
	if err != nil {
		return
	}
	if v, ok := res[mib.StpForceVersion]; ok {
		cf.StpVersion = mib.StpVersionEnum.Name(int(sx.Int(v)))
	}
	cf.StpPriority = int(sx.U64(res[mib.StpCstBridgePriority]))
	cf.BridgeID = fmt.Sprintf("%X", sx.Bytes(res[mib.StpMstBridgeIdentifier+".0"]))
	cf.RootID = fmt.Sprintf("%X", sx.Bytes(res[mib.StpMstDesignatedRootID+".0"]))
	cf.IsRoot = cf.BridgeID != "" && cf.BridgeID == cf.RootID
	tc := sx.U64(res[mib.StpMstTopoChangeCount+".0"])
	if d, ok := w.cnt.delta("tcn", tc, true); ok && d >= 5 {
		w.emit(model.Event{Category: "ANOMALY", Severity: model.SevWarning, Code: "STP_TCN_BURST", Title: fmt.Sprintf("STPトポロジー変更が急増（+%d）", d),
			Hint: "物理ループ・リンクのフラップの可能性。Show STPで破線リンクを確認し、対象ポートを切り分けてください"})
	}
	cf.TopoChanges = tc
	cf.Bonjour = sx.Int(res[mib.BonjourAdminMode]) == 1
	cf.BcastStormGlobal = sx.Int(res[mib.BroadcastControlModeGlobal]) == 1
	cf.McastStormGlobal = sx.Int(res[mib.MulticastControlModeGlobal]) == 1
	cf.HTTPS = sx.Int(res[mib.SSLAdminMode]) == 1
	cf.HTTPSPort = int(sx.Int(res[mib.SSLSecurePort]))
	cf.SSH = sx.Int(res[mib.SSHAdminMode]) == 1
	cf.SSHPort = int(sx.Int(res[mib.SSHMgmtPortNum]))
	cf.TelnetPort = int(sx.Int(res[mib.TelnetMgmtPortNum]))
	if ix := int(sx.U64(res[mib.CableTesterIfIndex])); ix > 0 {
		if p := pm[ix]; p != nil {
			p.Cable = &model.Cable{Status: mib.CableStatusEnum.Name(int(sx.Int(res[mib.CableTesterCableStatus]))),
				MinLen: int(sx.U64(res[mib.CableTesterMinLen])), MaxLen: int(sx.U64(res[mib.CableTesterMaxLen])), FailLoc: int(sx.U64(res[mib.CableTesterFailLoc]))}
		}
	}
	perPort := func(cols []string, f func(p *model.Port, r sx.Row)) {
		if rows, err := w.c.WalkColumns(cols); err == nil {
			for _, r := range rows {
				if p := pm[ifIdx(r)]; p != nil {
					f(p, r)
				}
			}
		}
	}
	perPort([]string{mib.GreenEeeMode}, func(p *model.Port, r sx.Row) { p.EEE = mib.GreenModeEnum.Name(int(sx.Int(r.Cols[mib.GreenEeeMode]))) })
	perPort([]string{mib.UdldIntfOperStatus}, func(p *model.Port, r sx.Row) {
		p.UDLD = mib.UdldOperEnum.Name(int(sx.Int(r.Cols[mib.UdldIntfOperStatus])))
	})
	perPort([]string{mib.StpCstPortFwdState}, func(p *model.Port, r sx.Row) {
		p.StpState = mib.StpFwdStateEnum.Name(int(sx.Int(r.Cols[mib.StpCstPortFwdState])))
	})
	perPort([]string{mib.CosMapIntfTrustMode}, func(p *model.Port, r sx.Row) {
		p.Trust = mib.TrustModeEnum.Name(int(sx.Int(r.Cols[mib.CosMapIntfTrustMode])))
	})
	// agentPortConfigTable（INDEX=basePort。PortCfgIfIndex列でifIndexへ変換）
	if rows, err := w.c.WalkColumns([]string{mib.PortCfgIfIndex, mib.PortCfgMaxFrameSize, mib.PortCfgBcastCtrlMode, mib.PortCfgMcastCtrlMode}); err == nil {
		for _, r := range rows {
			if p := pm[int(sx.Int(r.Cols[mib.PortCfgIfIndex]))]; p != nil {
				p.MaxFrame = int(sx.Int(r.Cols[mib.PortCfgMaxFrameSize]))
				p.BcastCtl = sx.Int(r.Cols[mib.PortCfgBcastCtrlMode]) == 1
				p.McastCtl = sx.Int(r.Cols[mib.PortCfgMcastCtrlMode]) == 1
			}
		}
	}
	if w.supported("sfp") {
		perPort([]string{mib.OpticsInfoVendor, mib.OpticsInfoPartNo, mib.OpticsInfoSupported}, func(p *model.Port, r sx.Row) {
			if p.SFP == nil {
				p.SFP = &model.SFP{}
			}
			p.SFP.Vendor, p.SFP.PartNo, p.SFP.Supported = sx.Str(r.Cols[mib.OpticsInfoVendor]), sx.Str(r.Cols[mib.OpticsInfoPartNo]), sx.Str(r.Cols[mib.OpticsInfoSupported])
		})
	}
	// プロセス別CPU（DisplayString から5秒値を抽出）
	if rows, err := w.c.WalkColumns([]string{mib.CpuProcessName, mib.CpuProcessPct}); err == nil {
		h := &w.st.Health
		h.Processes = h.Processes[:0]
		for _, r := range rows {
			raw := sx.Str(r.Cols[mib.CpuProcessPct])
			pct := 0.0
			if m := regexp.MustCompile(`\(([\d.]+)%\)`).FindStringSubmatch(raw); m != nil {
				pct, _ = strconv.ParseFloat(m[1], 64)
			} else {
				pct = firstFloat(raw)
			}
			h.Processes = append(h.Processes, model.Process{Name: sx.Str(r.Cols[mib.CpuProcessName]), Pct: pct, Raw: raw})
		}
		sort.Slice(h.Processes, func(i, j int) bool { return h.Processes[i].Pct > h.Processes[j].Pct })
		if len(h.Processes) > 10 {
			h.Processes = h.Processes[:10]
		}
	}
}

// collectLogs は agentLogInMemoryTable を MsgIndex で差分取得する（重複排除）。
func (w *worker) collectLogs() {
	rows, err := w.c.WalkColumns([]string{mib.LogInMemoryMsgText})
	if err != nil {
		return
	}
	// スイッチ再起動でMsgIndexが振り直された場合は基準を戻す
	top := 0
	for _, r := range rows {
		if ix := ifIdx(r); ix > top {
			top = ix
		}
	}
	if top < w.lastLogIdx {
		w.lastLogIdx = 0
	}
	maxIdx := w.lastLogIdx
	for _, r := range rows {
		ix := ifIdx(r)
		if ix <= w.lastLogIdx {
			continue
		}
		if ix > maxIdx {
			maxIdx = ix
		}
		text := sx.Str(r.Cols[mib.LogInMemoryMsgText])
		// 監視PC起動前のログも取り込む。アプリ再起動時の重複はDedupeキーでDB側が排除する
		w.emit(model.Event{Category: "SYSLOG", Severity: model.SevInfo, Code: "SWITCH_LOG", Title: text,
			Detail: map[string]any{"msg_index": ix}, Dedupe: fmt.Sprintf("log:%d:%s", ix, text)})
	}
	w.lastLogIdx = maxIdx
}

var _ = gosnmp.Integer
