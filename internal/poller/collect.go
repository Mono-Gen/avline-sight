package poller

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/oui"
	"github.com/Mono-Gen/avline-sight/internal/mcast"
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

var (
	discardCounters   = map[string]string{"ifInDiscards": mib.IfInDiscards, "ifOutDiscards": mib.IfOutDiscards}
	etherlikeCounters = map[string]string{
		"dot3StatsFCSErrors": mib.Dot3FCSErrors, "dot3StatsAlignmentErrors": mib.Dot3AlignmentErrors,
		"dot3StatsSymbolErrors": mib.Dot3SymbolErrors, "dot3StatsFrameTooLongs": mib.Dot3FrameTooLongs,
		"dot3StatsLateCollisions": mib.Dot3LateCollisions,
	}
	cosCounters = map[string]string{"agentCosQueueIntfCounterHCTotalDropPackets": mib.CosQueueIntfHCTotalDrop}
	poeCounters = map[string]string{
		"pethPsePortOverLoadCounter": mib.PethPortOverLoadCnt, "pethPsePortPowerDeniedCounter": mib.PethPortPowerDeniedCnt,
		"pethPsePortShortCounter": mib.PethPortShortCnt,
	}
)

func captureCounters(raw map[string]string, cols map[string]gosnmp.SnmpPDU, objects map[string]string) map[string]string {
	for name, oid := range objects {
		v := cols[oid]
		if !isRawCounter(v) {
			delete(raw, name)
			continue
		}
		if raw == nil {
			raw = make(map[string]string)
		}
		raw[name] = strconv.FormatUint(sx.U64(v), 10)
	}
	return raw
}

func isRawCounter(v gosnmp.SnmpPDU) bool {
	return v.Value != nil && (v.Type == gosnmp.Counter32 || v.Type == gosnmp.Counter64)
}

func indexedRawCounters(rows []sx.Row, oid, name string) map[string]string {
	raw := make(map[string]string)
	for _, row := range rows {
		v := row.Cols[oid]
		if !isRawCounter(v) {
			continue
		}
		instance := name
		for _, index := range row.Index {
			instance += "." + strconv.FormatUint(uint64(index), 10)
		}
		raw[instance] = strconv.FormatUint(sx.U64(v), 10)
	}
	return raw
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
			Title:  "スイッチ再起動（sysUpTime巻き戻り）を検知",
			Detail: map[string]any{"prev_ticks": w.lastUptime, "cur_ticks": ticks}})
		w.cnt.vals = map[string]uint64{} // カウンタ基準をリセット
		w.ifLastChange = nil
		w.flapChange = nil
	}
	w.lastUptime = ticks
	w.uptimeAt = time.Now() // ifLastChange（同じsysUpTime基準）を監視PCの時刻へ直すために、取得した時刻を覚える
	st.UptimeSec = up
	return nil
}

// linkChangeTime は ifLastChange（sysUpTime基準の 1/100 秒）を監視PCの時刻に直す。
// uptimeTicks は uptimeAt の時点で取得した sysUpTime。まだ取っていない、または変化が未来になる（再起動直後で古い値）ときは false。
func linkChangeTime(now, uptimeAt time.Time, uptimeTicks int64, changeTicks uint64) (time.Time, bool) {
	if uptimeAt.IsZero() || uptimeTicks <= 0 {
		return time.Time{}, false
	}
	nowTicks := uptimeTicks + now.Sub(uptimeAt).Milliseconds()/10
	if int64(changeTicks) > nowTicks {
		return time.Time{}, false
	}
	return now.Add(-time.Duration(nowTicks-int64(changeTicks)) * 10 * time.Millisecond).UTC(), true
}

// collectFast はポート状態・流量・PoE実電力（Fast Tier）。
func (w *worker) collectFast() error {
	now := time.Now()
	dt := now.Sub(w.cnt.ts).Seconds()
	first := w.cnt.ts.IsZero()
	trapLinks := w.requests.takeLinkSignals()
	rows, err := w.c.WalkColumns([]string{mib.IfName, mib.IfType, mib.IfOperStatus, mib.IfLastChange, mib.IfHighSpeed, mib.IfAlias,
		mib.IfHCInOctets, mib.IfHCOutOctets, mib.IfHCInMulticastPkts, mib.IfHCOutMulticastPkts, mib.IfHCInBroadcastPkts,
		mib.IfInErrors, mib.IfOutErrors, mib.IfInDiscards, mib.IfOutDiscards})
	if err != nil {
		w.requests.restoreLinkSignals(trapLinks)
		return err
	}
	ports := map[int]*model.Port{}
	for _, p := range w.st.Ports {
		ports[p.IfIndex] = p
	}
	var list []*model.Port
	lastChanges := make(map[int]uint64, len(rows))
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
		w.recordSetting(p, "alias", p.Alias)
		if ifType == 161 || strings.HasPrefix(strings.ToLower(name), "lag") {
			p.Type = "lag"
		} else if p.Type == "" || p.Type == "lag" {
			p.Type = "rj45" // SFP判定はMedium（DOM応答有無）で上書き
		}
		oper := mib.IfOperStatusEnum.Name(int(sx.Int(r.Cols[mib.IfOperStatus])))
		change := r.Cols[mib.IfLastChange]
		if change.Type == gosnmp.TimeTicks {
			lastChanges[ix] = sx.U64(change)
		}
		w.observeLinkFlap(p, oper, trapLinks[ix], lastChanges)
		spd := int(sx.Int(r.Cols[mib.IfHighSpeed]))
		if !first && p.Oper != "" && (p.Oper != oper || (oper == "up" && p.SpeedMbps != spd && p.SpeedMbps != 0)) {
			w.onLinkChange(p, oper, spd)
		}
		p.Oper, p.SpeedMbps = oper, spd
		// 最後にリンクが変化した時刻を、スイッチ自身の ifLastChange から監視PCの時刻で求める（スイッチの時計がずれていても正しい）。
		// アプリ起動前の変化も分かる。sysUpTime をまだ取っていない、または値が合わない（再起動直後など）ときは前の値のまま。
		if t, ok := lastChanges[ix]; ok {
			if at, ok := linkChangeTime(now, w.uptimeAt, w.lastUptime, t); ok {
				p.LastChange = at
			}
		}
		if oper == "up" && spd > 0 {
			defer func(ix, sp int) { w.lastUpSpeed[ix] = sp }(ix, spd)
		}
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
		p.RawCounters = captureCounters(p.RawCounters, r.Cols, discardCounters)
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
	w.ifLastChange = lastChanges
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
			p.RawCounters = captureCounters(p.RawCounters, r.Cols, poeCounters)
			if prev != nil {
				w.onPoEChange(p, prev, poe, r.Cols)
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
			d := func(col string, is32 bool) uint64 {
				v, _ := w.cnt.delta("m"+col+"|"+k, sx.U64(r.Cols[col]), is32)
				return v
			}
			p.FCSDelta = d(mib.Dot3FCSErrors, true)
			p.AlignDelta = d(mib.Dot3AlignmentErrors, true)
			p.SymbolDelta = d(mib.Dot3SymbolErrors, true)
			p.TooLongDelta = d(mib.Dot3FrameTooLongs, true)
			p.LateColDelta = d(mib.Dot3LateCollisions, true)
			p.RawCounters = captureCounters(p.RawCounters, r.Cols, etherlikeCounters)
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
				p.RawCounters = captureCounters(p.RawCounters, r.Cols, cosCounters)
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
		w.st.MbufFailCounters = indexedRawCounters(rows, mib.MbufAllocFails, "agentSwitchMbufAllocFails")
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
	// 取れなかったときは OFF と区別する（OFF と誤判定して「Snooping全体OFF」を誤報しないため）
	mode, known := res[mib.SnoopingAdminMode+".1"]
	ig.SnoopingKnown = known
	ig.SnoopingOn = known && sx.Int(mode) == 1
	vrows, err := w.c.WalkColumns([]string{mib.SnoopVlanAdminMode, mib.SnoopVlanFastLeave})
	if err != nil {
		return err
	}
	qrows, qerr := w.c.WalkColumns([]string{mib.QuerierVlanAdminMode, mib.QuerierVlanOperMode, mib.QuerierVlanAddress, mib.QuerierOperVersion, mib.QuerierLastQuerierAddress, mib.QuerierLastQuerierVersion})
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
	// nil は「まだ読んでいない」の印（エンジンがクエリア不在の判定を待つ）。読めたら行が無くても空スライスにする
	// クエリア表を読めなかったときは、まだ一度も読めていなければ「未収集」のまま残す（誤って不在と判定しない）
	if qerr != nil && ig.Vlans == nil {
		return nil
	}
	if ig.Vlans == nil {
		ig.Vlans = []model.VlanIGMP{}
	}
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
		g := agg[k]
		// 推定IPと用途を添える。L2（MAC）しか取れないときはプロトコル名も辞書の値にする
		info := mcast.Lookup(g.Group)
		g.IP, g.IPGuessed, g.PurposeJa, g.PurposeEn = info.IP, info.Guessed, info.PurposeJa, info.PurposeEn
		if g.Source == "l2" && info.Protocol != "Unknown" {
			g.Protocol = info.Protocol
		}
		ig.Groups = append(ig.Groups, *g)
	}
	mrows, _ := w.c.WalkColumns([]string{mib.VlanStaticMrouterAdminMode})
	ig.Mrouters = ig.Mrouters[:0]
	for _, r := range mrows {
		if len(r.Index) >= 3 && sx.Int(r.Cols[mib.VlanStaticMrouterAdminMode]) == 1 {
			ig.Mrouters = append(ig.Mrouters, model.Mrouter{Port: pname(int(r.Index[0])), VLAN: int(r.Index[1])})
		}
	}
	w.collectMVRMSRP(pname)
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

var msrpDecl = mib.Enum{0: "talkerAdvertise", 1: "talkerFailed", 2: "listenerAskingFailed", 3: "listenerReady", 4: "listenerReadyFailed"}

// collectMVRMSRP は MVRグループ/所属ポート/IGMP受信失敗 と MSRPストリーム/予約失敗 を取得する（仕様3.3-7,8）。
// 未有効・未対応のスイッチでは行が0件となり、UIは自動で折りたたむ。
func (w *worker) collectMVRMSRP(pname func(int) string) {
	ig := &w.st.IGMP
	ig.MVRGroups = ig.MVRGroups[:0]
	if w.supported("mvr") {
		mres, merr := w.c.Get([]string{mib.MvrAdminMode})
		_, hasMvr := mres[mib.MvrAdminMode]
		cnt := 0
		if hasMvr {
			cnt = 1
		}
		w.markCap("mvr", merr, cnt)
		grows, _ := w.c.WalkColumns([]string{mib.MvrGroupStatus})
		prows, perr := w.c.WalkColumns([]string{mib.MvrPortMemberRow})
		byG := map[string]*model.MVRGroup{}
		var order []string
		for _, r := range grows {
			g := sx.IndexIPv4(r.Index)
			byG[g] = &model.MVRGroup{Group: g, Active: sx.Int(r.Cols[mib.MvrGroupStatus]) == 1}
			order = append(order, g)
		}
		if perr == nil {
			for _, r := range prows {
				if len(r.Index) < 5 {
					continue
				}
				g := sx.IndexIPv4(r.Index[:4])
				if byG[g] == nil {
					byG[g] = &model.MVRGroup{Group: g}
					order = append(order, g)
				}
				byG[g].Ports = append(byG[g].Ports, pname(int(r.Index[4])))
			}
		}
		for _, g := range order {
			ig.MVRGroups = append(ig.MVRGroups, *byG[g])
		}
		if res, err := w.c.Get([]string{mib.MvrIGMPRxFailures}); err == nil {
			if v, ok := res[mib.MvrIGMPRxFailures]; ok {
				n := sx.U64(v)
				ig.MVRRxFailDelta, _ = w.cnt.delta("mvrrx", n, true)
				ig.MVRRxFailures = n
			}
		}
	}
	ig.MSRPStreams, ig.MSRPReservations, ig.MSRPFailed = 0, 0, ig.MSRPFailed[:0]
	if w.supported("msrp") {
		srows, err := w.c.WalkColumns([]string{mib.MsrpStreamID})
		rrows, rerr := w.c.WalkColumns([]string{mib.MsrpResvDeclType, mib.MsrpResvFailureCode})
		w.markCap("msrp", err, len(srows)+len(rrows))
		ig.MSRPStreams = len(srows)
		if rerr == nil {
			ig.MSRPReservations = len(rrows)
			for _, r := range rrows {
				decl := int(sx.Int(r.Cols[mib.MsrpResvDeclType]))
				code := int(sx.Int(r.Cols[mib.MsrpResvFailureCode]))
				if decl == 1 || decl == 2 || decl == 4 || code != 0 {
					// INDEX = streamId(8オクテット), direction, port
					sid, port := "", ""
					if len(r.Index) >= 10 {
						var b []byte
						for _, x := range r.Index[:8] {
							b = append(b, byte(x))
						}
						sid = fmt.Sprintf("%X", b)
						port = pname(int(r.Index[9]))
					}
					ig.MSRPFailed = append(ig.MSRPFailed, model.MSRPFailure{StreamID: sid, Port: port, Decl: msrpDecl.Name(decl), Code: code})
				}
			}
		}
	}
}

func (w *worker) basePortToIf(bp int) int {
	if ix, ok := w.basePort[bp]; ok {
		return ix
	}
	return bp
}

// collectSlow は接続機器・DHCP・ログ・設定（Slow Tier）。
func (w *worker) collectSlow() error {
	// 非対応と判定した機能グループも定期的に再確認する（運用中にMVR/AVB等を有効化されても追従するため）
	w.slowCount++
	if w.slowCount%reprobeEverySlow == 0 {
		for k, v := range w.st.Caps {
			if v == "UNSUPPORTED" {
				delete(w.st.Caps, k)
			}
		}
	}
	if err := w.collectSystem(); err != nil {
		return err
	}
	w.collectClock()
	pm := w.portMap()
	// Bridge Port→ifIndex 変換表（仕様4.2: 必ず経由）
	if rows, err := w.c.WalkColumns([]string{mib.Dot1dBasePortIfIdx}); err == nil {
		w.basePort = map[int]int{}
		for _, r := range rows {
			w.basePort[ifIdx(r)] = int(sx.Int(r.Cols[mib.Dot1dBasePortIfIdx]))
		}
	} else {
		w.markConfigFailed("pvid")
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
				w.recordPositiveSetting(p, "pvid", p.PVID)
			}
		}
	} else {
		w.markConfigFailed("pvid")
	}
	w.collectVlanMembership(pm)

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
	if entries, err := w.collectFDB(); err == nil {
		for _, entry := range entries {
			ix := w.basePortToIf(entry.port)
			if p := pm[ix]; p == nil || p.Type == "lag" {
				continue
			}
			add(ix, entry.mac).VLAN = entry.vlan
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
	// subtype列は同じ表の整数1つずつなので、追加してもリクエスト数はほぼ増えない。
	lrows, err := w.c.WalkColumns([]string{mib.LldpRemChassisIDSubtype, mib.LldpRemChassisID, mib.LldpRemPortIDSubtype, mib.LldpRemPortID, mib.LldpRemSysName, mib.LldpRemSysDesc})
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
		chs, _ := lldpID(r.Cols[mib.LldpRemChassisID], sx.Int(r.Cols[mib.LldpRemChassisIDSubtype]), lldpChassisMAC, true)
		portID, portIsMAC := lldpID(r.Cols[mib.LldpRemPortID], sx.Int(r.Cols[mib.LldpRemPortIDSubtype]), lldpPortMAC, false)
		n := model.Neighbor{LocalIfIndex: local, ChassisID: chs, SysName: sx.Str(r.Cols[mib.LldpRemSysName]),
			SysDesc: sx.Str(r.Cols[mib.LldpRemSysDesc]), PortID: portID,
			MgmtIP: mgmt[fmt.Sprintf("%d.%d", r.Index[1], r.Index[2])]}
		n.IsNetgear = strings.Contains(strings.ToUpper(n.SysDesc), "NETGEAR") || strings.Contains(n.SysDesc, "M4250") || strings.Contains(n.SysDesc, "AV Line") || oui.Lookup(chs) == "Netgear"
		if p := pm[local]; p != nil {
			n.LocalPort = p.Name
			if key := lldpDeviceKey(chs, portID, portIsMAC); key != "" {
				d := add(local, key)
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
		w.st.DHCPInvalidServerCounters = indexedRawCounters(rows, mib.DhcpSnoopInvalidServer, "agentDhcpSnoopingInvalidServerMessages")
	}
}

func (w *worker) collectConfig(pm map[int]*model.Port) {
	cf := &w.st.Config
	res, err := w.c.Get([]string{mib.StpForceVersion, mib.StpCstBridgePriority, mib.StpMstBridgeIdentifier + ".0", mib.StpMstDesignatedRootID + ".0",
		mib.StpMstTopoChangeCount + ".0", mib.BonjourAdminMode, mib.BroadcastControlModeGlobal, mib.MulticastControlModeGlobal,
		mib.SSLAdminMode, mib.SSLSecurePort, mib.SSHAdminMode, mib.SSHMgmtPortNum, mib.TelnetMgmtPortNum,
		mib.CableTesterIfIndex, mib.CableTesterCableStatus, mib.CableTesterMinLen, mib.CableTesterMaxLen, mib.CableTesterFailLoc})
	if err != nil {
		w.markConfigFailed("eee", "trust", "max_frame", "bcast_storm_ctl", "mcast_storm_ctl")
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
			Detail: map[string]any{"raw_counters": map[string]string{"agentDot1sMstTopologyChangeCount.0": strconv.FormatUint(tc, 10)}}})
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
	perPort := func(cols []string, f func(p *model.Port, r sx.Row)) error {
		rows, err := w.c.WalkColumns(cols)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if p := pm[ifIdx(r)]; p != nil {
				f(p, r)
			}
		}
		return nil
	}
	if err := perPort([]string{mib.GreenEeeMode}, func(p *model.Port, r sx.Row) {
		if v := r.Cols[mib.GreenEeeMode]; v.Type == gosnmp.Integer && v.Value != nil {
			p.EEE = mib.GreenModeEnum.Name(int(sx.Int(v)))
			if p.EEE != "not-applicable" {
				w.recordSetting(p, "eee", mib.GreenModeEnum[int(sx.Int(v))])
			}
		}
	}); err != nil {
		w.markConfigFailed("eee")
	}
	perPort([]string{mib.UdldIntfOperStatus}, func(p *model.Port, r sx.Row) {
		p.UDLD = mib.UdldOperEnum.Name(int(sx.Int(r.Cols[mib.UdldIntfOperStatus])))
	})
	// LAG の所属（標準 IEEE8023-LAG-MIB。値は所属する LAG の ifIndex、未所属は 0）。
	// 読めたときだけ全ポートを一度クリアしてから入れ直す（抜けたポートに古い所属を残さない）。
	if rows, err := w.c.WalkColumns([]string{mib.Dot3adAggPortAttached}); err == nil {
		for _, p := range pm {
			p.LagIfIndex, p.LagName = 0, ""
		}
		for _, r := range rows {
			p := pm[ifIdx(r)]
			agg := int(sx.Int(r.Cols[mib.Dot3adAggPortAttached]))
			if p == nil || agg <= 0 || agg == p.IfIndex {
				continue
			}
			p.LagIfIndex = agg
			if lag := pm[agg]; lag != nil {
				p.LagName = lag.Name
			}
		}
	}
	perPort([]string{mib.StpCstPortFwdState}, func(p *model.Port, r sx.Row) {
		p.StpState = mib.StpFwdStateEnum.Name(int(sx.Int(r.Cols[mib.StpCstPortFwdState])))
	})
	if err := perPort([]string{mib.CosMapIntfTrustMode}, func(p *model.Port, r sx.Row) {
		p.Trust = mib.TrustModeEnum.Name(int(sx.Int(r.Cols[mib.CosMapIntfTrustMode])))
		w.recordSetting(p, "trust", mib.TrustModeEnum[int(sx.Int(r.Cols[mib.CosMapIntfTrustMode]))])
	}); err != nil {
		w.markConfigFailed("trust")
	}
	// agentPortConfigTable（INDEX=basePort。PortCfgIfIndex列でifIndexへ変換）
	if rows, err := w.c.WalkColumns([]string{mib.PortCfgIfIndex, mib.PortCfgMaxFrameSize, mib.PortCfgBcastCtrlMode, mib.PortCfgMcastCtrlMode}); err == nil {
		for _, r := range rows {
			if p := pm[int(sx.Int(r.Cols[mib.PortCfgIfIndex]))]; p != nil {
				p.MaxFrame = int(sx.Int(r.Cols[mib.PortCfgMaxFrameSize]))
				w.recordPositiveSetting(p, "max_frame", p.MaxFrame)
				p.BcastCtl = sx.Int(r.Cols[mib.PortCfgBcastCtrlMode]) == 1
				p.McastCtl = sx.Int(r.Cols[mib.PortCfgMcastCtrlMode]) == 1
				w.recordStormSetting(p, "bcast_storm_ctl", sx.Int(r.Cols[mib.PortCfgBcastCtrlMode]))
				w.recordStormSetting(p, "mcast_storm_ctl", sx.Int(r.Cols[mib.PortCfgMcastCtrlMode]))
			}
		}
	} else {
		w.markConfigFailed("max_frame", "bcast_storm_ctl", "mcast_storm_ctl")
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
		detail := map[string]any{"msg_index": ix}
		if !w.logPrimed {
			// 初回取得分はスイッチに溜まっていた過去のログ。受信時刻が今になるため、
			// 「異常・警告」の絞り込みで今起きた警告に見えないよう印を付ける（ログ一覧には残す）
			detail["backlog"] = true
		}
		w.emit(model.Event{Category: "SYSLOG", Severity: syslogSeverity(text), Code: "SWITCH_LOG", Title: text,
			Detail: detail, Dedupe: fmt.Sprintf("log:%d:%s", ix, text)})
	}
	w.lastLogIdx = maxIdx
	w.logPrimed = true
}

// syslogSeverity はスイッチ内部ログ先頭の PRI（例: "<188>"）から重要度を決める。
// syslog の重要度は PRI を 8 で割った余り（RFC 5424 6.2.1）。
// 0〜2（emerg/alert/crit）を CRITICAL、3〜4（err/warning）を WARNING、5〜7 と PRI 無し・不正は INFO とする。
// err(3) を CRITICAL にしないのは、スイッチの err は運用上の一過性メッセージも多く、
// 最重の表示が埋もれるのを避けるため。SYSLOG はアラートにならず（engine.go）、ログ画面の色と絞り込みだけに効く。
func syslogSeverity(text string) string {
	if !strings.HasPrefix(text, "<") {
		return model.SevInfo
	}
	end := strings.IndexByte(text, '>')
	// PRI は 1〜3 桁で、最大 191（facility 23 × 8 + 7）
	if end < 2 || end > 4 {
		return model.SevInfo
	}
	pri, err := strconv.Atoi(text[1:end])
	if err != nil || pri < 0 || pri > 191 {
		return model.SevInfo
	}
	switch sev := pri % 8; {
	case sev <= 2:
		return model.SevCritical
	case sev <= 4:
		return model.SevWarning
	default:
		return model.SevInfo
	}
}

var _ = gosnmp.Integer

// LLDPのID subtype（IEEE 802.1AB）。MACアドレスのときだけバイト列をMAC表記にする。
const (
	lldpChassisMAC = 4
	lldpPortMAC    = 3
)

// lldpID はLLDPのchassis ID / port IDを表示用の文字列にする。
// subtypeがMACならMAC表記。subtypeが読めない（0）ときは6バイトをMACとみなすが、
// assumeMACがfalseなら表示できない文字を含む場合に限る（6文字のポート名をMACと誤らないため）。
// 例: PCのLLDPはchassis IDにホスト名（local）、port IDにMACのバイト列を送ることがある。
func lldpID(pdu gosnmp.SnmpPDU, subtype, macSubtype int64, assumeMAC bool) (string, bool) {
	b := sx.Bytes(pdu)
	mac := subtype == macSubtype || (subtype == 0 && len(b) == 6 && (assumeMAC || !printable(b)))
	if mac {
		if text := sx.BytesToMAC(b); text != "" {
			return text, true
		}
	}
	return sx.Str(pdu), false
}

func printable(b []byte) bool {
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return false
		}
	}
	return true
}

// lldpDeviceKey は接続機器一覧での識別子。MACで識別できればFDB/ARPの同じ機器と1件にまとまる。
// chassis IDがMACでなく（ホスト名など）、port IDがMACならそちらを使う。
func lldpDeviceKey(chassis, portID string, portIsMAC bool) string {
	if isMACText(chassis) {
		return chassis
	}
	if portIsMAC {
		return portID
	}
	return chassis
}

func isMACText(s string) bool {
	_, err := net.ParseMAC(s)
	return err == nil && len(s) == 17
}

// collectVlanMembership はポートごとのVLANモードと、実際に流れているVLAN（tagged/untagged）を取る。
// 毎回空に戻してから埋める（取れなかったときに古い値を「今の構成」と誤認させないため）。
func (w *worker) collectVlanMembership(pm map[int]*model.Port) {
	for _, p := range pm {
		p.Mode, p.NativeVLAN, p.TaggedVLANs, p.UntaggedVLANs = "", 0, nil, nil
	}
	if rows, err := w.c.WalkColumns([]string{mib.SwitchportMode, mib.SwitchportNativeVlan}); err == nil {
		for _, r := range rows {
			if p := pm[ifIdx(r)]; p != nil {
				p.Mode = switchportModeName(sx.Int(r.Cols[mib.SwitchportMode]))
				p.NativeVLAN = int(sx.Int(r.Cols[mib.SwitchportNativeVlan]))
			}
		}
	}
	rows, err := w.c.WalkColumns([]string{mib.Dot1qVlanCurrentEgress, mib.Dot1qVlanCurrentUntagged})
	if err != nil {
		return
	}
	for _, r := range rows {
		if len(r.Index) == 0 {
			continue
		}
		vlan := int(r.Index[len(r.Index)-1]) // INDEX=timeMark,vlan
		untagged := map[int]bool{}
		for _, bp := range sx.PortListBits(sx.Bytes(r.Cols[mib.Dot1qVlanCurrentUntagged])) {
			untagged[bp] = true
		}
		for _, bp := range sx.PortListBits(sx.Bytes(r.Cols[mib.Dot1qVlanCurrentEgress])) {
			p := pm[w.basePortToIf(bp)]
			if p == nil {
				continue
			}
			if untagged[bp] {
				p.UntaggedVLANs = append(p.UntaggedVLANs, vlan)
			} else {
				p.TaggedVLANs = append(p.TaggedVLANs, vlan)
			}
		}
	}
	for _, p := range pm {
		sort.Ints(p.TaggedVLANs)
		sort.Ints(p.UntaggedVLANs)
	}
}

// switchportModeName は agentSwitchportMode の値を名前にする（FASTPATH系MIB: access(1) trunk(2) general(3)）。
func switchportModeName(v int64) string {
	switch v {
	case 1:
		return "access"
	case 2:
		return "trunk"
	case 3:
		return "general"
	}
	return ""
}
