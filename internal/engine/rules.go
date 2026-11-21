package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// ルールベース診断の原因候補ヒント（仕様3.6の表）
const (
	hintPhy      = "物理ケーブル不良・コネクタ接触不良・ノイズ混入・SFP光劣化の疑い（公式基準: 正常は完全ゼロ。1件でも要確認）"
	hintDiscard  = "帯域輻輳・Storm Control干渉・QoSキュー溢れの疑い（10分間観測で傾向確認推奨）"
	hintMTU      = "接続機器または対向スイッチとのMTU（ジャンボフレーム / Q-SYS MTU 1500等）不一致の疑い"
	hintSpeed    = "LANケーブル芯線断線（4芯落ち）・コネクタ圧着不良・オートネゴ不整合の疑い"
	hintCos      = "特定ポートの帯域飽和・マイクロバースト・QoS優先度設定の要確認"
	hintPause    = "Flow Control作動によるストリーム遅延の恐れ。AV用途では無効化推奨（NDI4除く）"
	hintEEE      = "省電力イーサネット(802.3az)が有効。Dante/PTP同期不安定・音切れの原因"
	hintSFP      = "光ファイバ端面汚れ・曲げ損失・断線・モジュール不良（SFP清掃・再挿抜推奨）"
	hintUDLD     = "光ファイバ片芯断線・Tx/Rx異常"
	hintDuplex   = "Duplex不整合（片側固定・片側オート）の疑い"
	hintChassis  = "スイッチ過負荷・冷却異常・電源瞬断/再起動"
	hintDHCP     = "不正ルーター持ち込み・二重DHCP・固定IPとの重複"
	hintPoE      = "PoE電力不足または受電機器側の異常・短絡の疑い"
	hintStp      = "物理ループ発生（Show STPで破線リンク確認・対象ポート切り分け）またはIGMP+非推奨のMSTP設定"
	hintQuerier  = "最小IPルールによるエッジ選出、または外部機器（例：Crestron PRO4等）がクエリ送信しフラッディングさせている可能性"
	hintNoQuery  = "クエリア不在によりマルチキャストが1〜5分後に途絶する恐れ。コアスイッチでIGMP Querierを有効化してください"
	hintFlood    = "Unknown Multicastフラッディング発生の疑い（Snooping/Querier確認）"
	hintFW       = "ネットワーク不安定の主要因（公式トラブルシュート Step 1）。バージョン統一を推奨"
	hintStpPrio  = "全スイッチが既定優先度32768のままだとエッジがRoot Bridgeになり得ます。推奨: コア=0(Primary)、予備コア=8192(Backup)"
)

// ndi4系はFlow Control必要（公式ガイド）。PAUSE警告を出し分ける
func pauseAllowed(role string) bool { return role == "ndi4" }

func isAVRole(r string) bool {
	switch r {
	case "dante", "aes67", "ndi", "ndi4", "sdvoe", "st2110", "qsys", "avb", "lighting":
		return true
	}
	return false
}

// evaluate はスイッチ単体のアラート判定（立ち上がりで1回発報、解消で解除）。
func (e *Engine) evaluate(st *model.SwitchState, tier string, set Settings) {
	th := set.Thresholds
	seen := map[string]bool{}
	sw := itoa(int(st.ID))
	ev := func(sev, code, title, hint string, ix int, det map[string]any) model.Event {
		return model.Event{SwitchID: st.ID, SwitchName: st.Name, IfIndex: ix, Category: "ANOMALY", Severity: sev, Code: code, Title: title, Hint: hint, Detail: det}
	}
	key := func(sev, code string, ix int) string { return sw + "|" + sev + "|" + code + "|" + itoa(ix) + "|" + tier }

	if tier == "fast" || tier == "medium" {
		for _, p := range st.Ports {
			if p.Excluded || p.Oper != "up" {
				continue
			}
			nm := p.Name
			if tier == "fast" {
				if th.UtilPct > 0 && (p.UtilRx >= th.UtilPct || p.UtilTx >= th.UtilPct) {
					e.raise(seen, key(model.SevWarning, "HIGH_UTIL", p.IfIndex), ev(model.SevWarning, "HIGH_UTIL", fmt.Sprintf("%s 帯域使用率 %.0f%%", nm, maxf(p.UtilRx, p.UtilTx)), hintDiscard, p.IfIndex, nil))
				}
				if p.DiscardDelta > 0 {
					e.raise(seen, key(model.SevWarning, "DISCARDS", p.IfIndex), ev(model.SevWarning, "DISCARDS", fmt.Sprintf("%s パケットドロップ +%d", nm, p.DiscardDelta), hintDiscard, p.IfIndex, map[string]any{"delta": p.DiscardDelta}))
				}
				if p.ExpectedSpeed > 0 && p.SpeedMbps > 0 && p.SpeedMbps < p.ExpectedSpeed {
					e.raise(seen, key(model.SevCritical, "SPEED_BELOW_EXPECTED", p.IfIndex), ev(model.SevCritical, "SPEED_BELOW_EXPECTED", fmt.Sprintf("%s 期待速度未満 %dM < %dM", nm, p.SpeedMbps, p.ExpectedSpeed), hintSpeed, p.IfIndex, nil))
				}
				if p.PoE != nil && p.PoE.Fault != "" && p.PoE.Fault != "none" {
					e.raise(seen, key(model.SevCritical, "POE_PORT_FAULT", p.IfIndex), ev(model.SevCritical, "POE_PORT_FAULT", fmt.Sprintf("%s PoE障害 %s", nm, p.PoE.Fault), hintPoE, p.IfIndex, nil))
				}
				continue
			}
			// medium
			phy := p.FCSDelta + p.SymbolDelta + p.AlignDelta
			if th.ErrDelta > 0 && phy >= th.ErrDelta {
				e.raise(seen, key(model.SevCritical, "PHY_ERRORS", p.IfIndex), ev(model.SevCritical, "PHY_ERRORS", fmt.Sprintf("%s 物理エラー増加 FCS+%d Symbol+%d Align+%d", nm, p.FCSDelta, p.SymbolDelta, p.AlignDelta), hintPhy, p.IfIndex,
					map[string]any{"fcs": p.FCSDelta, "symbol": p.SymbolDelta, "align": p.AlignDelta}))
			}
			if p.TooLongDelta > 0 {
				e.raise(seen, key(model.SevWarning, "MTU_MISMATCH", p.IfIndex), ev(model.SevWarning, "MTU_MISMATCH", fmt.Sprintf("%s FrameTooLongs +%d", nm, p.TooLongDelta), hintMTU, p.IfIndex, nil))
			}
			if p.LateColDelta > 0 || p.Duplex == "halfDuplex" {
				e.raise(seen, key(model.SevWarning, "DUPLEX", p.IfIndex), ev(model.SevWarning, "DUPLEX", fmt.Sprintf("%s Half-Duplex/Late Collision", nm), hintDuplex, p.IfIndex, nil))
			}
			if p.CosDropDelta > 0 {
				e.raise(seen, key(model.SevCritical, "COS_DROP", p.IfIndex), ev(model.SevCritical, "COS_DROP", fmt.Sprintf("%s CoSキュー ドロップ +%d", nm, p.CosDropDelta), hintCos, p.IfIndex, nil))
			}
			if p.PauseDelta > 0 && !pauseAllowed(p.AVRole) {
				e.raise(seen, key(model.SevWarning, "PAUSE_FRAMES", p.IfIndex), ev(model.SevWarning, "PAUSE_FRAMES", fmt.Sprintf("%s PAUSEフレーム +%d", nm, p.PauseDelta), hintPause, p.IfIndex, nil))
			}
			avPort := isAVRole(p.AVRole) || (p.AVRole == "" && len(p.Devices) > 0)
			if avPort && (p.EEE == "enable" || p.LPIDelta > 0) {
				e.raise(seen, key(model.SevWarning, "EEE_ON_AV_PORT", p.IfIndex), ev(model.SevWarning, "EEE_ON_AV_PORT", fmt.Sprintf("%s EEE有効/LPI遷移（+%d）", nm, p.LPIDelta), hintEEE, p.IfIndex, nil))
			}
			if s := p.SFP; s != nil {
				bkey := sw + ":" + itoa(p.IfIndex)
				e.mu.RLock()
				base, hasBase := e.baseline[bkey]
				e.mu.RUnlock()
				switch {
				case s.LOS || s.TxFault:
					e.raise(seen, key(model.SevCritical, "SFP_LOS", p.IfIndex), ev(model.SevCritical, "SFP_LOS", fmt.Sprintf("%s SFP LOS/TxFault", nm), hintSFP, p.IfIndex, nil))
				case th.SFPRxDBm != 0 && s.RxDBm < th.SFPRxDBm && s.RxDBm > -40:
					e.raise(seen, key(model.SevWarning, "SFP_LOW_RX", p.IfIndex), ev(model.SevWarning, "SFP_LOW_RX", fmt.Sprintf("%s SFP受光 %.2f dBm（< %.1f）", nm, s.RxDBm, th.SFPRxDBm), hintSFP, p.IfIndex, map[string]any{"rx_dbm": s.RxDBm}))
				case hasBase && th.SFPDropDB > 0 && base-s.RxDBm >= th.SFPDropDB:
					e.raise(seen, key(model.SevWarning, "SFP_RX_DROP", p.IfIndex), ev(model.SevWarning, "SFP_RX_DROP", fmt.Sprintf("%s SFP受光がベースライン比 -%.1fdB", nm, base-s.RxDBm), hintSFP, p.IfIndex, nil))
				}
			}
		}
		if tier == "medium" {
			h := st.Health
			if th.CPUPct > 0 && h.CPU5s >= th.CPUPct {
				e.raise(seen, key(model.SevWarning, "CPU_HIGH", 0), ev(model.SevWarning, "CPU_HIGH", fmt.Sprintf("CPU使用率 %.0f%%", h.CPU5s), hintChassis, 0, nil))
			}
			if th.MemFreePct > 0 && h.MemFreePct > 0 && h.MemFreePct <= th.MemFreePct {
				e.raise(seen, key(model.SevWarning, "MEM_LOW", 0), ev(model.SevWarning, "MEM_LOW", fmt.Sprintf("空きメモリ %.0f%%", h.MemFreePct), hintChassis, 0, nil))
			}
			if (th.TempC > 0 && h.TempMaxC >= th.TempC) || h.TempState == "warning" || h.TempState == "critical" || h.TempState == "shutdown" {
				sev := model.SevWarning
				if h.TempState == "critical" || h.TempState == "shutdown" {
					sev = model.SevCritical
				}
				e.raise(seen, key(sev, "TEMP_HIGH", 0), ev(sev, "TEMP_HIGH", fmt.Sprintf("温度 %.0f℃（%s）", h.TempMaxC, h.TempState), hintChassis, 0, nil))
			}
			for _, f := range h.Fans {
				if f.State == "failed" || f.State == "nopower" || f.State == "notpowering" || f.State == "incompatible" {
					e.raise(seen, key(model.SevCritical, "FAN_FAIL", f.Index), ev(model.SevCritical, "FAN_FAIL", fmt.Sprintf("ファン%d 異常（%s）", f.Index, f.State), hintChassis, 0, nil))
				}
			}
			for _, f := range h.PSUs {
				if f.State == "failed" || f.State == "nopower" || f.State == "notpowering" || f.State == "incompatible" {
					e.raise(seen, key(model.SevCritical, "PSU_FAIL", f.Index), ev(model.SevCritical, "PSU_FAIL", fmt.Sprintf("電源%d 異常（%s）", f.Index, f.State), hintChassis, 0, nil))
				}
			}
			if h.MbufFailDelta > 0 {
				e.raise(seen, key(model.SevCritical, "MBUF_ALLOC_FAIL", 0), ev(model.SevCritical, "MBUF_ALLOC_FAIL", fmt.Sprintf("Mbuf確保失敗 +%d", h.MbufFailDelta), hintCos, 0, nil))
			}
			if th.PoEPct > 0 && h.PoEBudgetW > 0 && h.PoEUsedW/h.PoEBudgetW*100 >= th.PoEPct {
				e.raise(seen, key(model.SevWarning, "POE_BUDGET", 0), ev(model.SevWarning, "POE_BUDGET", fmt.Sprintf("PoEバジェット使用率 %.0f%%", h.PoEUsedW/h.PoEBudgetW*100), hintPoE, 0, nil))
			}
		}
	}
	if tier == "slow" {
		for _, p := range st.Ports {
			if p.Excluded {
				continue
			}
			if p.UDLD == "shutdown" {
				e.raise(seen, key(model.SevCritical, "UDLD", p.IfIndex), ev(model.SevCritical, "UDLD", fmt.Sprintf("%s UDLD片方向障害", p.Name), hintUDLD, p.IfIndex, nil))
			}
			if p.StpState == "discarding" && p.Oper == "up" {
				e.raise(seen, key(model.SevWarning, "STP_BLOCKING", p.IfIndex), ev(model.SevWarning, "STP_BLOCKING", fmt.Sprintf("%s STP Blocking", p.Name), hintStp, p.IfIndex, nil))
			}
		}
		if st.Config.StpVersion == "mstp" {
			e.raise(seen, key(model.SevWarning, "STP_MSTP", 0), ev(model.SevWarning, "STP_MSTP", "MSTPが稼働中（ProAVではRSTP推奨）", hintStp, 0, nil))
		}
		if st.DHCP.InvalidDelta > 0 {
			e.raise(seen, key(model.SevCritical, "DHCP_ROGUE", 0), ev(model.SevCritical, "DHCP_ROGUE", fmt.Sprintf("不正DHCPサーバーメッセージ +%d", st.DHCP.InvalidDelta), hintDHCP, 0, nil))
		}
		if len(st.DHCP.Conflicts) > 0 {
			e.raise(seen, key(model.SevCritical, "IP_CONFLICT", 0), ev(model.SevCritical, "IP_CONFLICT", "IPアドレス衝突: "+strings.Join(st.DHCP.Conflicts, ", "), hintDHCP, 0, nil))
		}
	}
	// 同じTierで今回成立しなかったキーのみ解除する（他Tierのアラートは維持）
	e.mu.Lock()
	for k := range e.active {
		if strings.HasPrefix(k, sw+"|") && strings.HasSuffix(k, "|"+tier) && !seen[k] {
			delete(e.active, k)
		}
	}
	e.mu.Unlock()
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// Finding は監査結果1件。
type Finding struct {
	Severity   string `json:"severity"`
	Code       string `json:"code"`
	Title      string `json:"title"`
	Hint       string `json:"hint"`
	SwitchID   int64  `json:"switch_id"`
	SwitchName string `json:"switch_name"`
	Port       string `json:"port,omitempty"`
}

// Audit はスイッチ間設定ズレ＆ProAV基準チェック（仕様3.5）。
func (e *Engine) Audit() ([]Finding, map[string][]string) {
	states := e.States()
	var fs []Finding
	fw := map[string][]string{}
	add := func(sev, code, title, hint string, s *model.SwitchState, port string) {
		f := Finding{Severity: sev, Code: code, Title: title, Hint: hint, Port: port}
		if s != nil {
			f.SwitchID, f.SwitchName = s.ID, s.Name
		}
		fs = append(fs, f)
	}
	var live []*model.SwitchState
	for _, s := range states {
		// 設定（Slow周期）まで取得済みのスイッチのみ比較対象にする（起動直後の過渡的な誤検知防止）
		if s.Reachable && s.FW != "" && s.Config.StpVersion != "" {
			live = append(live, s)
			fw[s.FW] = append(fw[s.FW], s.Name)
		}
	}
	if len(fw) > 1 {
		var parts []string
		for v, names := range fw {
			parts = append(parts, v+": "+strings.Join(names, ","))
		}
		sort.Strings(parts)
		add(model.SevWarning, "FW_MISMATCH", "ファームウェアバージョン不一致（"+strings.Join(parts, " / ")+"）", hintFW, nil, "")
	}
	// STP
	allDefault := len(live) > 1
	for _, s := range live {
		if s.Config.StpPriority != 32768 {
			allDefault = false
		}
		if s.Config.StpVersion == "mstp" {
			add(model.SevWarning, "STP_MSTP", "MSTP稼働（RSTP推奨、公式ガイドp.82）", hintStp, s, "")
		}
	}
	if allDefault {
		add(model.SevWarning, "STP_PRIORITY_DEFAULT", "全スイッチのSTP優先度が既定値32768", hintStpPrio, nil, "")
	}
	// ルートがエッジ（アップリンク1本以下）になっていないか
	for _, s := range live {
		if s.Config.IsRoot && len(live) > 1 && countUplinks(s) <= 1 {
			add(model.SevWarning, "STP_EDGE_ROOT", "エッジスイッチと思われる機器がSTP Root Bridge", hintStpPrio, s, "")
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
		add(model.SevCritical, "DHCP_DOUBLE", "複数スイッチでDHCPサーバーが稼働: "+strings.Join(dhcp, ", "), hintDHCP, nil, "")
	}
	// IGMP整合性・クエリア
	type vinfo struct {
		snoop, nosnoop []string
		queriers       []string
		lastQ          map[string]bool
		vers           map[int]bool
	}
	vl := map[int]*vinfo{}
	for _, s := range live {
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
				if !s.Config.IsRoot && countUplinks(s) <= 1 && len(live) > 1 {
					add(model.SevWarning, "QUERIER_ON_EDGE", fmt.Sprintf("VLAN%d エッジスイッチがクエリアに選出", v.VLAN), hintQuerier, s, "")
				}
			}
			if v.LastQuerier != "" {
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
			add(model.SevWarning, "SNOOP_MISMATCH", fmt.Sprintf("VLAN%d IGMP Snooping設定不一致（OFF: %s）", v, strings.Join(vi.nosnoop, ",")), hintFlood, nil, "")
		}
		if len(vi.snoop) > 0 && len(vi.queriers) == 0 && len(vi.lastQ) == 0 {
			add(model.SevCritical, "QUERIER_ABSENT", fmt.Sprintf("VLAN%d IGMPクエリア不在", v), hintNoQuery, nil, "")
		}
		if len(vi.lastQ) > 1 {
			var qs []string
			for q := range vi.lastQ {
				qs = append(qs, q)
			}
			sort.Strings(qs)
			add(model.SevCritical, "QUERIER_CONFLICT", fmt.Sprintf("VLAN%d 複数のクエリアIPを観測（%s）", v, strings.Join(qs, ", ")), hintQuerier, nil, "")
		}
		if len(vi.vers) > 1 {
			add(model.SevCritical, "IGMP_VERSION_MIX", fmt.Sprintf("VLAN%d IGMPバージョン混在", v), hintQuerier, nil, "")
		}
	}
	// ポート単位のAV地雷・リンク両端MTU
	for _, s := range live {
		for _, p := range s.Ports {
			if p.Excluded || p.Oper != "up" {
				continue
			}
			av := isAVRole(p.AVRole) || (p.AVRole == "" && len(p.Devices) > 0 && !isUplink(s, p))
			if av && p.EEE == "enable" {
				add(model.SevWarning, "EEE_ON_AV_PORT", "AVポートでEEE有効", hintEEE, s, p.Name)
			}
			if av && p.Pause != "" && p.Pause != "disabled" && !pauseAllowed(p.AVRole) {
				add(model.SevWarning, "FLOWCTRL_ON_AV_PORT", "AVポートでFlow Control(PAUSE)有効", hintPause, s, p.Name)
			}
			if p.AVRole == "ndi4" && (p.Pause == "" || p.Pause == "disabled") {
				add(model.SevWarning, "NDI4_NEEDS_FLOWCTRL", "NDI4ポートでFlow Control無効（公式ガイド上NDI4は有効化が必要）", "NDI4はFlow Controlを有効にしてください", s, p.Name)
			}
			if av && p.Trust != "" && p.Trust != "trustIpDscp" {
				add(model.SevInfo, "DSCP_TRUST", "AVポートのQoS TrustがDSCPではない（"+p.Trust+"）", "Dante/AES67/PTPのDSCP優先制御にはTrust DSCPを推奨", s, p.Name)
			}
			if (p.McastCtl || p.BcastCtl) && isUplink(s, p) {
				add(model.SevWarning, "STORM_CTRL_UPLINK", "アップリンクでStorm Control有効", "高帯域マルチキャストや gigaACE/CobraNet のアップリンクで不要なドロップ原因になり得ます（公式ガイドp.14, p.71）", s, p.Name)
			}
			if p.AVRole == "qsys" && p.MaxFrame > 1522 {
				add(model.SevWarning, "QSYS_MTU", "Q-SYSポートでMTU≠1500（最大フレーム "+itoa(p.MaxFrame)+"）", hintMTU, s, p.Name)
			}
		}
	}
	// LLDPで繋がるNETGEAR同士の両端MaxFrame比較
	byName := map[string]*model.SwitchState{}
	for _, s := range live {
		byName[s.SysName] = s
	}
	for _, s := range live {
		for _, n := range s.Neighbors {
			peer := byName[n.SysName]
			if peer == nil || peer.ID < s.ID {
				continue
			}
			lp, rp := findPort(s, n.LocalPort), findPort(peer, n.PortID)
			if lp != nil && rp != nil && lp.MaxFrame > 0 && rp.MaxFrame > 0 && lp.MaxFrame != rp.MaxFrame {
				add(model.SevWarning, "UPLINK_MTU_MISMATCH", fmt.Sprintf("アップリンク両端のMTU不一致 %s:%s=%d / %s:%s=%d", s.Name, lp.Name, lp.MaxFrame, peer.Name, rp.Name, rp.MaxFrame), hintMTU, s, lp.Name)
			}
		}
	}
	// トポロジー構造（デイジーチェーン検出: 3台以上で中継スイッチの連鎖）
	if depth := chainDepth(live); depth >= 3 {
		add(model.SevInfo, "DAISY_CHAIN", fmt.Sprintf("多段デイジーチェーン構成（最大%d段）", depth), "SMPTE / AES67 / PTPv2 はスター型必須。コアから各エッジへ直結する構成を推奨", nil, "")
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
			add(model.SevWarning, "VLAN_CROWDED", fmt.Sprintf("VLAN%d に%d台（200台超）", v, n), "Q-SYS等は1VLANあたり200台以下を推奨。VLAN分割を検討", nil, "")
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

func isUplink(s *model.SwitchState, p *model.Port) bool {
	if p.AVRole == "uplink" || p.Type == "lag" {
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
		if p.Name == name {
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
	fs, _ := e.Audit()
	seen := map[string]bool{}
	for _, f := range fs {
		// スイッチ単体ルールで既に発報するものは重複させない
		if f.Severity == model.SevInfo || f.Code == "STP_MSTP" || f.Code == "EEE_ON_AV_PORT" {
			continue
		}
		k := "0|" + f.Severity + "|AUDIT|" + f.Code + "|" + f.SwitchName + "|" + f.Port + "|" + f.Title
		e.raise(seen, k, model.Event{SwitchID: f.SwitchID, SwitchName: f.SwitchName, Category: "ANOMALY", Severity: f.Severity, Code: f.Code, Title: f.Title, Hint: f.Hint})
	}
	// クエリアIPの変化（VLAN単位）
	for _, s := range e.States() {
		for _, v := range s.IGMP.Vlans {
			k := itoa(int(s.ID)) + ":" + itoa(v.VLAN)
			e.mu.Lock()
			prev, ok := e.prevQuerier[k]
			e.prevQuerier[k] = v.LastQuerier
			e.mu.Unlock()
			if ok && prev != "" && v.LastQuerier != "" && prev != v.LastQuerier {
				e.OnEvent(model.Event{SwitchID: s.ID, SwitchName: s.Name, Category: "ANOMALY", Severity: model.SevCritical, Code: "QUERIER_CHANGED",
					Title: fmt.Sprintf("VLAN%d クエリアIP変化 %s → %s", v.VLAN, prev, v.LastQuerier), Hint: hintQuerier})
			}
		}
		// Unknown Multicastフラッディング: 多数ポートで送信Mcast ppsが高く、グループ登録が少ない
		// 仕様3.6: 「Mcast急増 かつ IGMPグループ未登録」。登録済みグループの正規配信は除外する
		joined := map[string]bool{}
		for _, g := range s.IGMP.Groups {
			for _, pn := range g.Ports {
				joined[pn] = true
			}
		}
		high := 0
		for _, p := range s.Ports {
			if p.Oper == "up" && p.TxMcPps > 20000 && !joined[p.Name] {
				high++
			}
		}
		k := itoa(int(s.ID)) + "|" + model.SevCritical + "|MCAST_FLOOD|0|x"
		if high >= 5 && high*2 >= len(s.Ports)/2 {
			e.raise(seen, k, model.Event{SwitchID: s.ID, SwitchName: s.Name, Category: "ANOMALY", Severity: model.SevCritical, Code: "MCAST_FLOOD",
				Title: fmt.Sprintf("%dポートでマルチキャスト急増（フラッディングの疑い）", high), Hint: hintFlood})
		} else {
			seen[k] = false
			e.mu.Lock()
			delete(e.active, k)
			e.mu.Unlock()
		}
	}
	e.clearPrefix("0|", seen)
	_ = set
}
