package engine

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// realQuerier は観測したクエリアIPが実在のアドレスかを返す。0.0.0.0 と :: はM4250の既定値
// （set igmp querier N address 0.0.0.0 ＝ アドレス未指定）で、クエリアがいることを意味しない（2026-11-25 実機の設定で確認）。
// これを実在扱いすると、別スイッチの実アドレスと並んで「複数クエリア」と誤検知し、本当の不在を見逃す。
func realQuerier(addr string) bool {
	return addr != "" && addr != "0.0.0.0" && addr != "::"
}

func isAVRole(r string) bool {
	switch r {
	case "dante", "aes67", "ndi", "ndi4", "sdvoe", "st2110", "qsys", "avb", "lighting":
		return true
	}
	return false
}

func counterDetails(p *model.Port, detail map[string]any, names ...string) map[string]any {
	raw := map[string]string{}
	for _, name := range names {
		if value, ok := p.RawCounters[name]; ok {
			raw[name] = value
		}
	}
	if len(raw) == 0 {
		return detail
	}
	if detail == nil {
		detail = map[string]any{}
	}
	detail["raw_counters"] = raw
	return detail
}

func rawCounter(name string, value uint64) map[string]any {
	return map[string]any{"raw_counters": map[string]string{name: strconv.FormatUint(value, 10)}}
}

// evaluate はスイッチ単体のアラート判定（立ち上がりで1回発報、解消で解除）。
func (e *Engine) evaluate(st *model.SwitchState, tier string, set Settings) {
	th := set.Thresholds
	seen := map[string]bool{}
	sw := itoa(int(st.ID))
	ev := func(sev, code, title string, ix int, det map[string]any) model.Event {
		return model.Event{SwitchID: st.ID, SwitchName: st.Name, IfIndex: ix, Category: "ANOMALY", Severity: sev, Code: code, Title: title, Hint: model.EventHintJa(code), Detail: det}
	}
	key := func(sev, code string, ix int) string {
		return sw + "|" + sev + "|" + code + "|" + itoa(ix) + "|" + tier
	}

	if tier == "fast" || tier == "medium" {
		for _, p := range st.Ports {
			if p.Excluded || p.Oper != "up" {
				continue
			}
			nm := p.Name
			if tier == "fast" {
				if th.UtilPct > 0 && (p.UtilRx >= th.UtilPct || p.UtilTx >= th.UtilPct) {
					e.raise(seen, key(model.SevWarning, "HIGH_UTIL", p.IfIndex), ev(model.SevWarning, "HIGH_UTIL", fmt.Sprintf("%s 帯域使用率 %.0f%%", nm, maxf(p.UtilRx, p.UtilTx)), p.IfIndex, nil))
				}
				if p.DiscardDelta > 0 {
					e.raise(seen, key(model.SevWarning, "DISCARDS", p.IfIndex), ev(model.SevWarning, "DISCARDS", fmt.Sprintf("%s パケットドロップ +%d", nm, p.DiscardDelta), p.IfIndex,
						counterDetails(p, map[string]any{"delta": p.DiscardDelta}, "ifInDiscards", "ifOutDiscards")))
				}
				if p.ExpectedSpeed > 0 && p.SpeedMbps > 0 && p.SpeedMbps < p.ExpectedSpeed {
					e.raise(seen, key(model.SevCritical, "SPEED_BELOW_EXPECTED", p.IfIndex), ev(model.SevCritical, "SPEED_BELOW_EXPECTED", fmt.Sprintf("%s 期待速度未満 %dM < %dM", nm, p.SpeedMbps, p.ExpectedSpeed), p.IfIndex, nil))
				}
				if p.PoE != nil && p.PoE.Fault != "" && p.PoE.Fault != "none" {
					e.raise(seen, key(model.SevCritical, "POE_PORT_FAULT", p.IfIndex), ev(model.SevCritical, "POE_PORT_FAULT", fmt.Sprintf("%s PoE障害 %s", nm, p.PoE.Fault), p.IfIndex,
						counterDetails(p, nil, "pethPsePortOverLoadCounter", "pethPsePortPowerDeniedCounter", "pethPsePortShortCounter")))
				}
				continue
			}
			// medium
			phy := p.FCSDelta + p.SymbolDelta + p.AlignDelta
			if th.ErrDelta > 0 && phy >= th.ErrDelta {
				e.raise(seen, key(model.SevCritical, "PHY_ERRORS", p.IfIndex), ev(model.SevCritical, "PHY_ERRORS", fmt.Sprintf("%s 物理エラー増加 FCS+%d Symbol+%d Align+%d", nm, p.FCSDelta, p.SymbolDelta, p.AlignDelta), p.IfIndex,
					counterDetails(p, map[string]any{"fcs": p.FCSDelta, "symbol": p.SymbolDelta, "align": p.AlignDelta},
						"dot3StatsFCSErrors", "dot3StatsSymbolErrors", "dot3StatsAlignmentErrors")))
			}
			if p.TooLongDelta > 0 {
				e.raise(seen, key(model.SevWarning, "MTU_MISMATCH", p.IfIndex), ev(model.SevWarning, "MTU_MISMATCH", fmt.Sprintf("%s FrameTooLongs +%d", nm, p.TooLongDelta), p.IfIndex,
					counterDetails(p, nil, "dot3StatsFrameTooLongs")))
			}
			if p.LateColDelta > 0 || p.Duplex == "halfDuplex" {
				e.raise(seen, key(model.SevWarning, "DUPLEX", p.IfIndex), ev(model.SevWarning, "DUPLEX", fmt.Sprintf("%s Half-Duplex/Late Collision", nm), p.IfIndex,
					counterDetails(p, nil, "dot3StatsLateCollisions")))
			}
			if p.CosDropDelta > 0 {
				e.raise(seen, key(model.SevCritical, "COS_DROP", p.IfIndex), ev(model.SevCritical, "COS_DROP", fmt.Sprintf("%s CoSキュー ドロップ +%d", nm, p.CosDropDelta), p.IfIndex,
					counterDetails(p, nil, "agentCosQueueIntfCounterHCTotalDropPackets")))
			}
			if s := p.SFP; s != nil {
				bkey := sw + ":" + itoa(p.IfIndex)
				e.mu.RLock()
				base, hasBase := e.baseline[bkey]
				e.mu.RUnlock()
				switch {
				case s.LOS || s.TxFault:
					e.raise(seen, key(model.SevCritical, "SFP_LOS", p.IfIndex), ev(model.SevCritical, "SFP_LOS", fmt.Sprintf("%s SFP LOS/TxFault", nm), p.IfIndex, nil))
				case th.SFPRxDBm != 0 && s.RxDBm < th.SFPRxDBm && s.RxDBm > -40:
					e.raise(seen, key(model.SevWarning, "SFP_LOW_RX", p.IfIndex), ev(model.SevWarning, "SFP_LOW_RX", fmt.Sprintf("%s SFP受光 %.2f dBm（< %.1f）", nm, s.RxDBm, th.SFPRxDBm), p.IfIndex, map[string]any{"rx_dbm": s.RxDBm}))
				case hasBase && th.SFPDropDB > 0 && base-s.RxDBm >= th.SFPDropDB:
					e.raise(seen, key(model.SevWarning, "SFP_RX_DROP", p.IfIndex), ev(model.SevWarning, "SFP_RX_DROP", fmt.Sprintf("%s SFP受光がベースライン比 -%.1fdB", nm, base-s.RxDBm), p.IfIndex, nil))
				}
			}
		}
		if tier == "medium" {
			// MVR/MSRP はMedium周期で収集されるため同じ周期で判定する（取りこぼし防止）
			if n := len(st.IGMP.MSRPFailed); n > 0 {
				e.raise(seen, key(model.SevWarning, "MSRP_RESV_FAILED", 0), ev(model.SevWarning, "MSRP_RESV_FAILED", fmt.Sprintf("AVB/Milan 帯域予約失敗 %d件", n),
					0, nil))
			}
			if st.IGMP.MVRRxFailDelta > 0 {
				e.raise(seen, key(model.SevWarning, "MVR_RX_FAIL", 0), ev(model.SevWarning, "MVR_RX_FAIL", fmt.Sprintf("MVR IGMPパケット受信失敗 +%d", st.IGMP.MVRRxFailDelta),
					0, rawCounter("mvrIGMPPacketReceiveFailures", st.IGMP.MVRRxFailures)))
			}
			h := st.Health
			if th.CPUPct > 0 && h.CPU5s >= th.CPUPct {
				e.raise(seen, key(model.SevWarning, "CPU_HIGH", 0), ev(model.SevWarning, "CPU_HIGH", fmt.Sprintf("CPU使用率 %.0f%%", h.CPU5s), 0, nil))
			}
			if th.MemFreePct > 0 && h.MemFreePct > 0 && h.MemFreePct <= th.MemFreePct {
				e.raise(seen, key(model.SevWarning, "MEM_LOW", 0), ev(model.SevWarning, "MEM_LOW", fmt.Sprintf("空きメモリ %.0f%%", h.MemFreePct), 0, nil))
			}
			if (th.TempC > 0 && h.TempMaxC >= th.TempC) || h.TempState == "warning" || h.TempState == "critical" || h.TempState == "shutdown" {
				sev := model.SevWarning
				if h.TempState == "critical" || h.TempState == "shutdown" {
					sev = model.SevCritical
				}
				e.raise(seen, key(sev, "TEMP_HIGH", 0), ev(sev, "TEMP_HIGH", fmt.Sprintf("温度 %.0f℃（%s）", h.TempMaxC, h.TempState), 0, nil))
			}
			for _, f := range h.Fans {
				if f.State == "failed" || f.State == "nopower" || f.State == "notpowering" || f.State == "incompatible" {
					e.raise(seen, key(model.SevCritical, "FAN_FAIL", f.Index), ev(model.SevCritical, "FAN_FAIL", fmt.Sprintf("ファン%d 異常（%s）", f.Index, f.State), 0, nil))
				}
			}
			for _, f := range h.PSUs {
				if f.State == "failed" || f.State == "nopower" || f.State == "notpowering" || f.State == "incompatible" {
					e.raise(seen, key(model.SevCritical, "PSU_FAIL", f.Index), ev(model.SevCritical, "PSU_FAIL", fmt.Sprintf("電源%d 異常（%s）", f.Index, f.State), 0, nil))
				}
			}
			if h.MbufFailDelta > 0 {
				e.raise(seen, key(model.SevCritical, "MBUF_ALLOC_FAIL", 0), ev(model.SevCritical, "MBUF_ALLOC_FAIL", fmt.Sprintf("Mbuf確保失敗 +%d", h.MbufFailDelta), 0,
					map[string]any{"raw_counters": st.MbufFailCounters}))
			}
			if th.PoEPct > 0 && h.PoEBudgetW > 0 && h.PoEUsedW/h.PoEBudgetW*100 >= th.PoEPct {
				e.raise(seen, key(model.SevWarning, "POE_BUDGET", 0), ev(model.SevWarning, "POE_BUDGET", fmt.Sprintf("PoEバジェット使用率 %.0f%%", h.PoEUsedW/h.PoEBudgetW*100), 0, nil))
			}
		}
	}
	if tier == "slow" {
		if offset := st.ClockOffsetSeconds; offset != nil && math.Abs(*offset) > clockToleranceSeconds {
			e.raise(seen, key(model.SevWarning, "CLOCK_SKEW", 0), ev(model.SevWarning, "CLOCK_SKEW",
				fmt.Sprintf("監視PCとの時刻差 %.0f秒", *offset), 0,
				map[string]any{"offset_seconds": *offset}))
		}
		for _, p := range st.Ports {
			if p.Excluded {
				continue
			}
			if p.UDLD == "shutdown" {
				e.raise(seen, key(model.SevCritical, "UDLD", p.IfIndex), ev(model.SevCritical, "UDLD", fmt.Sprintf("%s UDLD片方向障害", p.Name), p.IfIndex, nil))
			}
			if p.StpState == "discarding" && p.Oper == "up" {
				e.raise(seen, key(model.SevWarning, "STP_BLOCKING", p.IfIndex), ev(model.SevWarning, "STP_BLOCKING", fmt.Sprintf("%s STP Blocking", p.Name), p.IfIndex, nil))
			}
		}
		if st.DHCP.InvalidDelta > 0 {
			e.raise(seen, key(model.SevCritical, "DHCP_ROGUE", 0), ev(model.SevCritical, "DHCP_ROGUE", fmt.Sprintf("不正DHCPサーバーメッセージ +%d", st.DHCP.InvalidDelta), 0,
				map[string]any{"raw_counters": st.DHCPInvalidServerCounters}))
		}
		if len(st.DHCP.Conflicts) > 0 {
			e.raise(seen, key(model.SevCritical, "IP_CONFLICT", 0), ev(model.SevCritical, "IP_CONFLICT", "IPアドレス衝突: "+strings.Join(st.DHCP.Conflicts, ", "), 0, nil))
		}
	}
	// 同じTierで今回成立しなかったキーのみ解除する（他Tierのアラートは維持）
	e.clearKeys(func(k string, a *Alert) bool {
		if a.Code == "CLOCK_SKEW" {
			return a.SwitchID == st.ID && tier == "slow" && st.ClockOffsetSeconds != nil &&
				math.Abs(*st.ClockOffsetSeconds) <= clockToleranceSeconds
		}
		return strings.HasPrefix(k, sw+"|") && strings.HasSuffix(k, "|"+tier) && !seen[k]
	})
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// Finding は監査結果1件。
type Finding struct {
	Severity   string         `json:"severity"`
	Code       string         `json:"code"`
	Title      string         `json:"title"`
	Hint       string         `json:"hint"`
	SwitchID   int64          `json:"switch_id"`
	SwitchName string         `json:"switch_name"`
	Port       string         `json:"port,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
}

// Audit はスイッチ間設定ズレ＆ProAV基準チェック（仕様3.5）。
// crossReady は監視中の全スイッチが一度は応答し設定まで取得済みか。
// 起動直後に一部のスイッチだけで比較すると「全台既定値」等を誤検知するため、揃うまで横断判定を保留する。
func crossReady(states []*model.SwitchState) bool {
	polled := false
	for _, s := range states {
		if s.NeedsCredentials {
			continue
		}
		if s.LastPoll.IsZero() || (s.Reachable && s.Config.StpVersion == "") {
			return false
		}
		polled = true
	}
	return polled
}

func (e *Engine) Audit() ([]Finding, map[string][]string) {
	states := e.States()
	var fs []Finding
	fw := map[string][]string{}
	if !crossReady(states) {
		return []Finding{{Severity: model.SevInfo, Code: "AUDIT_PENDING", Title: "全スイッチの初回取得待ちのため、横断監査を保留しています",
			Hint: model.EventHintJa("AUDIT_PENDING")}}, fw
	}
	add := func(sev, code, title string, s *model.SwitchState, port string, detail map[string]any) {
		if recommendationOnly(code) {
			sev = model.SevInfo
		}
		f := Finding{Severity: sev, Code: code, Title: title, Hint: model.EventHintJa(code), Port: port, Detail: detail}
		if s != nil {
			f.SwitchID, f.SwitchName = s.ID, s.Name
		}
		fs = append(fs, f)
	}
	var live []*model.SwitchState
	// ファームウェアは同じ機種どうしで比べる（機種が違えばFWの系列が違うのが普通。本体情報の表示と同じ規則）
	fwByModel := map[string]map[string][]string{}
	for _, s := range states {
		// 設定（Slow周期）まで取得済みのスイッチのみ比較対象にする（起動直後の過渡的な誤検知防止）
		if s.Reachable && s.FW != "" && s.Config.StpVersion != "" {
			live = append(live, s)
			fw[s.FW] = append(fw[s.FW], s.Name)
			if fwByModel[s.Model] == nil {
				fwByModel[s.Model] = map[string][]string{}
			}
			fwByModel[s.Model][s.FW] = append(fwByModel[s.Model][s.FW], s.Name)
		}
	}
	models := make([]string, 0, len(fwByModel))
	for m := range fwByModel {
		models = append(models, m)
	}
	sort.Strings(models)
	for _, m := range models {
		if len(fwByModel[m]) < 2 {
			continue
		}
		var parts []string
		for v, names := range fwByModel[m] {
			parts = append(parts, v+": "+strings.Join(names, ","))
		}
		sort.Strings(parts)
		versions := strings.Join(parts, " / ")
		if m != "" {
			versions = m + " — " + versions
		}
		add(model.SevWarning, "FW_MISMATCH", "ファームウェアバージョン不一致（"+versions+"）", nil, "", map[string]any{"versions": versions, "model": m})
	}
	// STP
	allDefault := len(live) > 1
	for _, s := range live {
		if s.Config.StpPriority != 32768 {
			allDefault = false
		}
		if s.Config.StpVersion == "mstp" {
			add(model.SevWarning, "STP_MSTP", "MSTP稼働（RSTP推奨、公式ガイドp.82）", s, "", nil)
		}
	}
	if allDefault {
		add(model.SevWarning, "STP_PRIORITY_DEFAULT", "全スイッチのSTP優先度が既定値32768", nil, "", nil)
	}
	// ルートがエッジ（アップリンク1本以下）になっていないか
	for _, s := range live {
		if s.Config.IsRoot && len(live) > 1 && countUplinks(s) <= 1 {
			add(model.SevWarning, "STP_EDGE_ROOT", "エッジスイッチと思われる機器がSTP Root Bridge", s, "", nil)
		}
	}
	// DHCP二重
	var dhcp []string
	for _, s := range live {
		if s.DHCP.ServerOn {
			dhcp = append(dhcp, s.Name)
		}
	}
	if len(dhcp) > 1 {
		add(model.SevCritical, "DHCP_DOUBLE", "複数スイッチでDHCPサーバーが稼働: "+strings.Join(dhcp, ", "), nil, "", map[string]any{"switches": strings.Join(dhcp, ", ")})
	}
	// IGMP整合性・クエリア
	type vinfo struct {
		snoop, nosnoop []string
		queriers       []string
		querierSw      []*model.SwitchState
		lastQ          map[string]bool
		vers           map[int]bool
	}
	vl := map[int]*vinfo{}
	for _, s := range live {
		// 全体のSnoopingがOFFだと、VLANごとのSnooping設定は効かず全ポートへフラッディングする。
		// VLANではSnoopingを使う設定になっているのに全体がOFF、という食い違いを重大として出す（取れなかったときは判定しない）。
		if s.IGMP.SnoopingKnown && !s.IGMP.SnoopingOn {
			var intended []string
			for _, v := range s.IGMP.Vlans {
				if v.Snooping {
					intended = append(intended, itoa(v.VLAN))
				}
			}
			if len(intended) > 0 {
				add(model.SevCritical, "SNOOP_GLOBAL_OFF", fmt.Sprintf("IGMP Snooping 全体設定がOFF（VLAN %s の設定が効いていない）", strings.Join(intended, ",")), s, "",
					map[string]any{"vlans": strings.Join(intended, ",")})
			}
		}
		for _, v := range s.IGMP.Vlans {
			vi := vl[v.VLAN]
			if vi == nil {
				vi = &vinfo{lastQ: map[string]bool{}, vers: map[int]bool{}}
				vl[v.VLAN] = vi
			}
			if v.Snooping {
				vi.snoop = append(vi.snoop, s.Name)
			} else {
				vi.nosnoop = append(vi.nosnoop, s.Name)
			}
			if v.QuerierOper == "querier" {
				vi.queriers = append(vi.queriers, s.Name)
				vi.querierSw = append(vi.querierSw, s)
				if !s.Config.IsRoot && countUplinks(s) <= 1 && len(live) > 1 {
					add(model.SevWarning, "QUERIER_ON_EDGE", fmt.Sprintf("VLAN%d エッジスイッチがクエリアに選出", v.VLAN), s, "", map[string]any{"vlan": v.VLAN})
				}
			}
			if realQuerier(v.LastQuerier) {
				vi.lastQ[v.LastQuerier] = true
			}
			if v.LastVersion > 0 {
				vi.vers[v.LastVersion] = true
			}
		}
	}
	vids := make([]int, 0, len(vl))
	for v := range vl {
		vids = append(vids, v)
	}
	sort.Ints(vids)
	for _, v := range vids {
		vi := vl[v]
		if len(vi.snoop) > 0 && len(vi.nosnoop) > 0 {
			add(model.SevWarning, "SNOOP_MISMATCH", fmt.Sprintf("VLAN%d IGMP Snooping設定不一致（OFF: %s）", v, strings.Join(vi.nosnoop, ",")), nil, "", map[string]any{"vlan": v})
		}
		if len(vi.snoop) > 0 && len(vi.queriers) == 0 && len(vi.lastQ) == 0 {
			add(model.SevCritical, "QUERIER_ABSENT", fmt.Sprintf("VLAN%d IGMPクエリア不在", v), nil, "", map[string]any{"vlan": v})
		}
		if len(vi.lastQ) > 1 {
			var qs []string
			for q := range vi.lastQ {
				qs = append(qs, q)
			}
			sort.Strings(qs)
			add(model.SevCritical, "QUERIER_CONFLICT", fmt.Sprintf("VLAN%d 複数のクエリアIPを観測（%s）", v, strings.Join(qs, ", ")), nil, "", map[string]any{"vlan": v, "queriers": strings.Join(qs, ", ")})
		} else {
			// アドレスが 0.0.0.0 でも、LLDPでつながった複数のスイッチがそれぞれ自分をクエリアと答えるのは選出の失敗
			// （互いのクエリを受けていない）。つながっていない別ネットワークで同じVLAN番号を使う構成は対象外にする。
			// つながったかたまりごとに1件ずつ、毎回同じ順で出す（順が変わると同じ問題が別のアラートとして出直すため）。
			for _, names := range connectedQueriers(live, vi.querierSw) {
				add(model.SevCritical, "QUERIER_CONFLICT", fmt.Sprintf("VLAN%d 複数のスイッチがクエリア（%s）", v, strings.Join(names, ", ")), nil, "", map[string]any{"vlan": v, "queriers": strings.Join(names, ", ")})
			}
		}
		if len(vi.vers) > 1 {
			add(model.SevCritical, "IGMP_VERSION_MIX", fmt.Sprintf("VLAN%d IGMPバージョン混在", v), nil, "", map[string]any{"vlan": v})
		}
	}
	// トポロジー構造（デイジーチェーン検出: 3台以上で中継スイッチの連鎖）
	if depth := chainDepth(live); depth >= 3 {
		add(model.SevInfo, "DAISY_CHAIN", fmt.Sprintf("多段デイジーチェーン構成（最大%d段）", depth), nil, "", map[string]any{"depth": depth})
	}
	// VLAN過密（Q-SYS: 1VLANあたり200台超）
	cnt := map[int]int{}
	for _, s := range live {
		for _, p := range s.Ports {
			if isUplink(s, p) {
				continue
			}
			for _, d := range p.Devices {
				cnt[d.VLAN]++
			}
		}
	}
	for v, n := range cnt {
		if n > 200 {
			add(model.SevWarning, "VLAN_CROWDED", fmt.Sprintf("VLAN%d に%d台（200台超）", v, n), nil, "", map[string]any{"vlan": v, "count": n})
		}
	}
	sort.SliceStable(fs, func(i, j int) bool { return sevRank(fs[i].Severity) < sevRank(fs[j].Severity) })
	if fs == nil {
		fs = []Finding{}
	}
	return fs, fw
}

func sevRank(s string) int {
	switch s {
	case model.SevCritical:
		return 0
	case model.SevWarning:
		return 1
	}
	return 2
}

// baseGroup は、ほぼ全機器が参加するため「このポートがストリームを受けている」根拠にならないグループ。
// 224.0.0.0/24（リンクローカル: mDNS 224.0.0.251 等）と PTP（224.0.1.129〜132）。
// M4250 はグループをMACアドレス（01:00:5E:…）でしか返さないことがある（実機 FW 13.0.7.6）ので両方の書式を見る。
// MAC書式は同じMACになる別のIPグループ（例 239.0.0.x）と区別できないため、mac=true を返して呼び出し側で扱いを分ける。
func baseGroup(group string) (base, mac bool) {
	g := strings.ToUpper(strings.TrimSpace(group))
	if strings.HasPrefix(g, "01:00:5E:") {
		switch {
		case strings.HasPrefix(g, "01:00:5E:00:00:"), g == "01:00:5E:00:01:81", g == "01:00:5E:00:01:82",
			g == "01:00:5E:00:01:83", g == "01:00:5E:00:01:84":
			return true, true
		}
		return false, true
	}
	switch g {
	case "224.0.1.129", "224.0.1.130", "224.0.1.131", "224.0.1.132":
		return true, false
	}
	return strings.HasPrefix(g, "224.0.0."), false
}

// floodMinPps 未満のマルチキャストは、フラッディングがあっても実害が小さく誤報の元なので判定しない
// （mDNS・PTPの告知などは毎秒数〜数十パケット）。
const floodMinPps = 500

// floodedPorts は、Snooping有効のVLANで、ストリームに参加していないポートへ流入マルチキャストが複製されている
// ポート数を返す（フラッディングでなければ0）。
//   - Snooping無効のVLANは全ポートへ配るのが正しい動作なので対象外。
//   - ほぼ全AV機器が参加する基本グループ（baseGroup）への参加は「受けている」根拠にしない（以前はこれで全ポートが対象外だった）。
//     ただしMAC書式は実ストリームと区別できないので、そのVLANのリンク中ポートの過半が参加しているときだけ基本グループと見なす
//     （PTP・mDNSはほぼ全機器が参加する。一部のポートだけなら実ストリームとして扱い、誤報しない側に倒す）。
//   - 流入量は、そのVLANのアクセスポートに加えてアップリンク・LAG（タグ付きで複数VLANを運ぶ）の受信も候補にする。
//     アップリンクは他VLANの流量も含むので多めに見積もる＝検知しにくい側に倒れる。
//   - 固定のpps閾値ではなく、流入量の半分以上が、参加していないポート（送信元自身は除く）の過半（かつ2ポート以上）へ
//     出ていればフラッディングとする（音声・照明のような少流量でも検知でき、規模に依存しない）。
func floodedPorts(s *model.SwitchState) int {
	// 全体がOFFなら全ポートへ配るのが今の正しい動作で、それ自体は SNOOP_GLOBAL_OFF で知らせる
	if s.IGMP.SnoopingKnown && !s.IGMP.SnoopingOn {
		return 0
	}
	snooping := map[int]bool{}
	for _, v := range s.IGMP.Vlans {
		snooping[v.VLAN] = v.Snooping
	}
	trunk := func(p *model.Port) bool { return p.Type == "lag" || isUplink(s, p) }
	access := map[int][]*model.Port{}
	accessName := map[string]bool{}
	var trunkRx float64
	for _, p := range s.Ports {
		if p.Oper != "up" {
			continue
		}
		if trunk(p) {
			trunkRx = max(trunkRx, p.RxMcPps)
			continue
		}
		access[p.PVID] = append(access[p.PVID], p)
		accessName[fmt.Sprintf("%d|%s", p.PVID, p.Name)] = true
	}
	joined := map[string]bool{}
	for _, g := range s.IGMP.Groups {
		if base, mac := baseGroup(g.Group); base {
			// MAC書式は、そのVLANのアクセスポートの過半が参加しているときだけ基本グループと見なす（トランクは数えない）
			members := 0
			for _, pn := range g.Ports {
				if accessName[fmt.Sprintf("%d|%s", g.VLAN, pn)] {
					members++
				}
			}
			if !mac || members*2 > len(access[g.VLAN]) {
				continue
			}
		}
		for _, pn := range g.Ports {
			joined[fmt.Sprintf("%d|%s", g.VLAN, pn)] = true
		}
	}
	// 流入量 source に対して、参加していないポートの過半（かつ2ポート以上）へ半分以上が出ていれば、そのポート数を返す。
	// 純粋な送信元（流入の半分以上を受け、出ているのは半分未満）は「参加していないポート」に数えない。
	// 参加していない送信機でもフラッディングを受けていれば（出ている量が半分以上）数える。
	check := func(vlan int, ports []*model.Port, source float64) int {
		if source < floodMinPps {
			return 0
		}
		high, candidates := 0, 0
		for _, p := range ports {
			if joined[fmt.Sprintf("%d|%s", vlan, p.Name)] {
				continue
			}
			receives := p.TxMcPps >= source/2
			if p.RxMcPps >= source/2 && !receives {
				continue
			}
			candidates++
			if receives {
				high++
			}
		}
		if high >= 2 && high*2 > candidates {
			return high
		}
		return 0
	}
	flooded := 0
	for vlan, ports := range access {
		if !snooping[vlan] {
			continue
		}
		var local float64
		for _, p := range ports {
			local = max(local, p.RxMcPps)
		}
		// 送信元がこのVLANのアクセスポートにある場合と、トランク（タグ付きで他VLANも運ぶ）から入る場合を別々に見る。
		// トランクの量を全VLANの基準にすると、少流量VLANのフラッディングを見逃すため。
		flooded += max(check(vlan, ports, local), check(vlan, ports, trunkRx))
	}
	return flooded
}

// connectedQueriers は、クエリアと答えたスイッチを LLDP（NETGEAR隣接）でつながったかたまりごとに分け、
// 2台以上いるかたまりの名前の一覧を、決まった順（名前順）で返す。
func connectedQueriers(live, queriers []*model.SwitchState) [][]string {
	if len(queriers) < 2 {
		return nil
	}
	parent := map[int64]int64{}
	var find func(int64) int64
	find = func(id int64) int64 {
		if p, ok := parent[id]; ok && p != id {
			root := find(p)
			parent[id] = root
			return root
		}
		parent[id] = id
		return id
	}
	byName := map[string]*model.SwitchState{}
	for _, s := range live {
		find(s.ID)
		if s.SysName != "" {
			byName[s.SysName] = s
		}
	}
	for _, s := range live {
		for _, n := range s.Neighbors {
			if peer := byName[n.SysName]; n.IsNetgear && peer != nil && peer.ID != s.ID {
				parent[find(s.ID)] = find(peer.ID)
			}
		}
	}
	groups := map[int64][]string{}
	for _, q := range queriers {
		root := find(q.ID)
		groups[root] = append(groups[root], q.Name)
	}
	var out [][]string
	for _, names := range groups {
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		out = append(out, names)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Join(out[i], ",") < strings.Join(out[j], ",") })
	return out
}

func isUplink(s *model.SwitchState, p *model.Port) bool {
	if p.AVRole == "uplink" || p.AVRole == "trunk" || p.Type == "lag" {
		return true
	}
	for _, n := range s.Neighbors {
		if n.LocalIfIndex == p.IfIndex && n.IsNetgear {
			return true
		}
	}
	return false
}

func countUplinks(s *model.SwitchState) int {
	n := 0
	for _, nb := range s.Neighbors {
		if nb.IsNetgear {
			n++
		}
	}
	return n
}

func findPort(s *model.SwitchState, name string) *model.Port {
	for _, p := range s.Ports {
		if p != nil && p.Name == name {
			return p
		}
	}
	return nil
}

// chainDepth はSTPルートからのNETGEARスイッチ段数の最大値。
func chainDepth(live []*model.SwitchState) int {
	byName := map[string]*model.SwitchState{}
	var root *model.SwitchState
	for _, s := range live {
		byName[s.SysName] = s
		if s.Config.IsRoot {
			root = s
		}
	}
	if root == nil {
		return 0
	}
	depth := map[string]int{root.SysName: 0}
	q := []*model.SwitchState{root}
	maxd := 0
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		for _, n := range c.Neighbors {
			if p := byName[n.SysName]; p != nil {
				if _, ok := depth[p.SysName]; !ok {
					depth[p.SysName] = depth[c.SysName] + 1
					if depth[p.SysName] > maxd {
						maxd = depth[p.SysName]
					}
					q = append(q, p)
				}
			}
		}
	}
	return maxd
}

// evaluateCross は横断ルールをイベント化する（監査結果のCRITICAL/WARNINGを立ち上がりで発報）。
func (e *Engine) evaluateCross(set Settings) {
	states := e.States()
	if !crossReady(states) {
		return
	}
	fs, _ := e.Audit()
	seen := map[string]bool{}
	for _, f := range fs {
		if f.Severity == model.SevInfo {
			continue
		}
		k := "0|" + f.Severity + "|AUDIT|" + f.Code + "|" + f.SwitchName + "|" + f.Port + "|" + f.Title
		e.raise(seen, k, model.Event{SwitchID: f.SwitchID, SwitchName: f.SwitchName, Category: "ANOMALY", Severity: f.Severity, Code: f.Code, Title: f.Title, Hint: f.Hint, Detail: f.Detail})
	}
	// クエリアIPの変化（VLAN単位）
	for _, s := range states {
		for _, v := range s.IGMP.Vlans {
			k := itoa(int(s.ID)) + ":" + itoa(v.VLAN)
			e.mu.Lock()
			prev, ok := e.prevQuerier[k]
			// 0.0.0.0（クエリア不在の間）で上書きしない。落ちて別のクエリアに替わったときも変化として出すため
			if realQuerier(v.LastQuerier) {
				e.prevQuerier[k] = v.LastQuerier
			}
			e.mu.Unlock()
			if ok && realQuerier(prev) && realQuerier(v.LastQuerier) && prev != v.LastQuerier {
				e.OnEvent(model.Event{SwitchID: s.ID, SwitchName: s.Name, Category: "ANOMALY", Severity: model.SevCritical, Code: "QUERIER_CHANGED",
					Title: fmt.Sprintf("VLAN%d クエリアIP変化 %s → %s", v.VLAN, prev, v.LastQuerier), Hint: model.EventHintJa("QUERIER_CHANGED"),
					Detail: map[string]any{"vlan": v.VLAN, "before": prev, "after": v.LastQuerier}})
			}
		}
		// Unknown Multicastフラッディング（仕様3.6「Mcast急増 かつ IGMPグループ未登録」）
		high := floodedPorts(s)
		k := itoa(int(s.ID)) + "|" + model.SevCritical + "|MCAST_FLOOD|0|x"
		if high > 0 {
			e.raise(seen, k, model.Event{SwitchID: s.ID, SwitchName: s.Name, Category: "ANOMALY", Severity: model.SevCritical, Code: "MCAST_FLOOD",
				Title: fmt.Sprintf("%dポートでマルチキャスト急増（フラッディングの疑い）", high), Hint: model.EventHintJa("MCAST_FLOOD")})
		} else {
			seen[k] = false
			e.clearKeys(func(key string, a *Alert) bool { return key == k })
		}
	}
	e.clearPrefix("0|", seen)
	_ = set
}
