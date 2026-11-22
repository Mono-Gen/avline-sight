package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// TopoNode / TopoEdge はトポロジーAPIの要素（docs/api.md）。
type TopoNode struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Kind      string   `json:"kind"` // switch / external
	SwitchID  int64    `json:"switch_id,omitempty"`
	Reachable bool     `json:"reachable"`
	IsRoot    bool     `json:"is_root"`
	X         *float64 `json:"x,omitempty"`
	Y         *float64 `json:"y,omitempty"`
	Detail    string   `json:"detail,omitempty"`
}

type TopoEdge struct {
	ID         string  `json:"id"`
	Source     string  `json:"source"`
	Target     string  `json:"target"`
	LocalPort  string  `json:"local_port"`
	RemotePort string  `json:"remote_port"`
	Blocking   bool    `json:"blocking"`
	SpeedMbps  int     `json:"speed_mbps"`
	UtilPct    float64 `json:"util_pct"`
}

type Topology struct {
	Nodes      []TopoNode `json:"nodes"`
	Edges      []TopoEdge `json:"edges"`
	Background *string    `json:"background"`
}

// Topology はLLDPからスイッチ間接続と他社隣接スイッチ（外部ノード）を組み立てる。
func (e *Engine) Topology(layout map[string]struct{ X, Y float64 }) Topology {
	states := e.States()
	t := Topology{Nodes: []TopoNode{}, Edges: []TopoEdge{}}
	byName, byMAC := map[string]*model.SwitchState{}, map[string]*model.SwitchState{}
	for _, s := range states {
		id := fmt.Sprintf("sw-%d", s.ID)
		n := TopoNode{ID: id, Label: s.Name, Kind: "switch", SwitchID: s.ID, Reachable: s.Reachable, IsRoot: s.Config.IsRoot, Detail: s.Model}
		if p, ok := layout[id]; ok {
			x, y := p.X, p.Y
			n.X, n.Y = &x, &y
		}
		t.Nodes = append(t.Nodes, n)
		if s.SysName != "" {
			byName[s.SysName] = s
		}
		if s.BaseMAC != "" {
			byMAC[s.BaseMAC] = s
		}
	}
	seenEdge := map[string]bool{}
	ext := map[string]bool{}
	for _, s := range states {
		for _, nb := range s.Neighbors {
			peer := byMAC[nb.ChassisID]
			if peer == nil {
				peer = byName[nb.SysName]
			}
			lp := findPort(s, nb.LocalPort)
			var src, dst string
			src = fmt.Sprintf("sw-%d", s.ID)
			switch {
			case peer != nil:
				dst = fmt.Sprintf("sw-%d", peer.ID)
			case isSwitchLike(nb):
				// 他社スイッチ（Cisco/Aruba/Yamaha等）は外部ノードとして表示
				dst = "ext-" + strings.ReplaceAll(nb.ChassisID, ":", "")
				if !ext[dst] {
					ext[dst] = true
					n := TopoNode{ID: dst, Label: firstNonEmpty(nb.SysName, nb.ChassisID), Kind: "external", Reachable: true, Detail: nb.SysDesc}
					if p, ok := layout[dst]; ok {
						x, y := p.X, p.Y
						n.X, n.Y = &x, &y
					}
					t.Nodes = append(t.Nodes, n)
				}
			default:
				continue // エンド機器はトポロジーには載せない（ポート詳細で表示）
			}
			a, b := src+"/"+nb.LocalPort, dst+"/"+nb.PortID
			k := a + "|" + b
			if a > b {
				k = b + "|" + a
			}
			if seenEdge[k] {
				// 反対側からも見えている: Blocking状態を合成
				for i := range t.Edges {
					if t.Edges[i].ID == k && lp != nil && lp.StpState == "discarding" {
						t.Edges[i].Blocking = true
					}
				}
				continue
			}
			seenEdge[k] = true
			ed := TopoEdge{ID: k, Source: src, Target: dst, LocalPort: nb.LocalPort, RemotePort: nb.PortID}
			if lp != nil {
				ed.Blocking = lp.StpState == "discarding"
				ed.SpeedMbps = lp.SpeedMbps
				ed.UtilPct = maxf(lp.UtilRx, lp.UtilTx)
			}
			t.Edges = append(t.Edges, ed)
		}
	}
	return t
}

func isSwitchLike(n model.Neighbor) bool {
	d := strings.ToLower(n.SysDesc + " " + n.SysName)
	for _, k := range []string{"switch", "cisco", "aruba", "catalyst", "nexus", "yamaha sw", "swr", "swx", "juniper", "extreme", "procurve", "dell", "hpe", "luminex", "netgear", "ios"} {
		if strings.Contains(d, k) {
			return true
		}
	}
	return n.IsNetgear
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

// LLDPCandidate は未登録の隣接NETGEARスイッチ（芋づる探索）。
type LLDPCandidate struct {
	IP        string `json:"ip"`
	SysName   string `json:"sys_name"`
	ViaSwitch string `json:"via_switch"`
	ViaPort   string `json:"via_port"`
}

func (e *Engine) LLDPCandidates(registeredIPs map[string]bool) []LLDPCandidate {
	known := map[string]bool{}
	for _, s := range e.States() {
		known[s.SysName] = true
	}
	out := []LLDPCandidate{}
	seen := map[string]bool{}
	for _, s := range e.States() {
		for _, n := range s.Neighbors {
			if !n.IsNetgear || n.MgmtIP == "" || registeredIPs[n.MgmtIP] || known[n.SysName] || seen[n.MgmtIP] {
				continue
			}
			seen[n.MgmtIP] = true
			out = append(out, LLDPCandidate{IP: n.MgmtIP, SysName: n.SysName, ViaSwitch: s.Name, ViaPort: n.LocalPort})
		}
	}
	return out
}

// Multicast はマルチキャスト・IGMP・AVB画面のデータ。
func (e *Engine) Multicast() map[string]any {
	var vlans, groups, mrouters, gptp []map[string]any
	for _, s := range e.States() {
		for _, v := range s.IGMP.Vlans {
			vlans = append(vlans, merge(v, s))
		}
		for _, g := range s.IGMP.Groups {
			groups = append(groups, merge(g, s))
		}
		for _, m := range s.IGMP.Mrouters {
			mrouters = append(mrouters, merge(m, s))
		}
		if s.IGMP.GPTP != nil {
			gptp = append(gptp, merge(*s.IGMP.GPTP, s))
		}
	}
	fs, _ := e.Audit()
	var mf []Finding
	for _, f := range fs {
		if strings.Contains(f.Code, "QUERIER") || strings.Contains(f.Code, "IGMP") || strings.Contains(f.Code, "SNOOP") || strings.Contains(f.Code, "MCAST") || strings.Contains(f.Code, "STORM") {
			mf = append(mf, f)
		}
	}
	return map[string]any{"vlans": nz(vlans), "groups": nz(groups), "mrouters": nz(mrouters), "gptp": nz(gptp), "findings": nzf(mf)}
}

// DHCP はDHCP監視画面のデータ。
func (e *Engine) DHCP() map[string]any {
	var servers, leases, conflicts []map[string]any
	for _, s := range e.States() {
		servers = append(servers, map[string]any{"switch_id": s.ID, "switch_name": s.Name, "server_on": s.DHCP.ServerOn, "pools": s.DHCP.Pools,
			"lease_count": len(s.DHCP.Leases), "decline_rx": s.DHCP.DeclineRx, "snooping_on": s.DHCP.SnoopingOn, "invalid_server_msgs": s.DHCP.InvalidServerMsgs})
		for _, l := range s.DHCP.Leases {
			leases = append(leases, merge(l, s))
		}
		for _, c := range s.DHCP.Conflicts {
			conflicts = append(conflicts, map[string]any{"switch_id": s.ID, "switch_name": s.Name, "ip": c})
		}
	}
	fs, _ := e.Audit()
	var df []Finding
	for _, f := range fs {
		if strings.HasPrefix(f.Code, "DHCP") || f.Code == "IP_CONFLICT" {
			df = append(df, f)
		}
	}
	for _, s := range e.States() {
		if s.DHCP.InvalidServerMsgs > 0 {
			df = append(df, Finding{Severity: model.SevCritical, Code: "DHCP_ROGUE", Title: fmt.Sprintf("不正DHCPサーバーメッセージ累計 %d", s.DHCP.InvalidServerMsgs), Hint: hintDHCP, SwitchID: s.ID, SwitchName: s.Name})
		}
		if len(s.DHCP.Conflicts) > 0 {
			df = append(df, Finding{Severity: model.SevCritical, Code: "IP_CONFLICT", Title: "IPアドレス衝突: " + strings.Join(s.DHCP.Conflicts, ", "), Hint: hintDHCP, SwitchID: s.ID, SwitchName: s.Name})
		}
	}
	return map[string]any{"servers": nz(servers), "leases": nz(leases), "conflicts": nz(conflicts), "findings": nzf(df)}
}

// merge は構造体をmapへ展開し switch_id / switch_name を付与する。
func merge(v any, s *model.SwitchState) map[string]any {
	m := toMap(v)
	m["switch_id"], m["switch_name"] = s.ID, s.Name
	return m
}

func nz(v []map[string]any) []map[string]any {
	if v == nil {
		return []map[string]any{}
	}
	return v
}

func nzf(v []Finding) []Finding {
	if v == nil {
		return []Finding{}
	}
	return v
}

// Diff はベースラインとの差分1件。
type Diff struct {
	SwitchID    int64  `json:"switch_id"`
	SwitchName  string `json:"switch_name"`
	Item        string `json:"item"`
	Before      string `json:"before"`
	After       string `json:"after"`
	Severity    string `json:"severity"`
	ConfigField string `json:"config_field,omitempty"`
}

// BaselineDiff は保存済みベースラインと現在値を比較する（仕様3.4）。
func (e *Engine) BaselineDiff(base []*model.SwitchState) []Diff {
	cur := map[int64]*model.SwitchState{}
	for _, s := range e.States() {
		cur[s.ID] = s
	}
	out := []Diff{}
	for _, b := range base {
		add := func(item, before, after, sev string) {
			if before != after {
				out = append(out, Diff{SwitchID: b.ID, SwitchName: b.Name, Item: item, Before: before, After: after, Severity: sev})
			}
		}
		c := cur[b.ID]
		if c == nil {
			add("スイッチ", "登録あり", "現在なし", model.SevWarning)
			continue
		}
		add("到達性", fmt.Sprint(b.Reachable), fmt.Sprint(c.Reachable), model.SevCritical)
		add("DHCPサーバー", onoff(b.DHCP.ServerOn), onoff(c.DHCP.ServerOn), model.SevWarning)
		add("STPルート", b.Config.RootID, c.Config.RootID, model.SevWarning)
		add("ファームウェア", b.FW, c.FW, model.SevInfo)
		cp := map[int]*model.Port{}
		for _, p := range c.Ports {
			cp[p.IfIndex] = p
		}
		for _, bp := range b.Ports {
			p := cp[bp.IfIndex]
			if p == nil {
				continue
			}
			n := "Port " + bp.Name
			add(n+": リンク", bp.Oper, p.Oper, model.SevWarning)
			if bp.Oper == "up" && p.Oper == "up" {
				add(n+": 速度", fmt.Sprintf("%dM", bp.SpeedMbps), fmt.Sprintf("%dM", p.SpeedMbps), model.SevCritical)
			}
			add(n+": 接続MAC", macs(bp), macs(p), model.SevWarning)
			add(n+": PoE給電", poeOn(bp), poeOn(p), model.SevWarning)
			if bp.SFP != nil && p.SFP != nil && bp.SFP.RxDBm-p.SFP.RxDBm >= 1 {
				add(n+": SFP Rx", fmt.Sprintf("%.2fdBm", bp.SFP.RxDBm), fmt.Sprintf("%.2fdBm (%.1fdB低下)", p.SFP.RxDBm, bp.SFP.RxDBm-p.SFP.RxDBm), model.SevWarning)
			}
			if !b.Reachable || !c.Reachable {
				continue
			}
			baselineSettings := bp.ConfigSettings
			if baselineSettings == nil {
				baselineSettings = legacyPortSettings(bp)
			}
			fields := make([]string, 0, len(baselineSettings))
			for field := range baselineSettings {
				fields = append(fields, field)
			}
			sort.Strings(fields)
			for _, field := range fields {
				before, after := baselineSettings[field], p.ConfigSettings[field]
				if before != "" && after != "" && before != after {
					out = append(out, Diff{SwitchID: b.ID, SwitchName: b.Name, Item: bp.Name, Before: before, After: after, Severity: model.SevInfo, ConfigField: field})
				}
			}
		}
		bv := map[int]model.VlanIGMP{}
		for _, v := range b.IGMP.Vlans {
			bv[v.VLAN] = v
		}
		for _, v := range c.IGMP.Vlans {
			if o, ok := bv[v.VLAN]; ok {
				add(fmt.Sprintf("VLAN %d: Querier IP", v.VLAN), o.LastQuerier, v.LastQuerier, model.SevCritical)
				add(fmt.Sprintf("VLAN %d: Snooping", v.VLAN), onoff(o.Snooping), onoff(v.Snooping), model.SevWarning)
			}
		}
		add("参加グループ数", fmt.Sprint(len(b.IGMP.Groups)), fmt.Sprint(len(c.IGMP.Groups)), model.SevInfo)
		if c.Health.TempMaxC-b.Health.TempMaxC >= 10 {
			add("温度", fmt.Sprintf("%.0f℃", b.Health.TempMaxC), fmt.Sprintf("%.0f℃", c.Health.TempMaxC), model.SevWarning)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return sevRank(out[i].Severity) < sevRank(out[j].Severity) })
	return out
}

func legacyPortSettings(p *model.Port) map[string]string {
	settings := make(map[string]string)
	if strings.TrimSpace(p.Alias) != "" {
		settings["alias"] = p.Alias
	}
	if p.PVID > 0 {
		settings["pvid"] = strconv.Itoa(p.PVID)
	}
	if p.MaxFrame > 0 {
		settings["max_frame"] = strconv.Itoa(p.MaxFrame)
	}
	if p.EEE == "enable" || p.EEE == "disable" {
		settings["eee"] = p.EEE
	}
	if p.Trust != "" && !strings.HasPrefix(p.Trust, "unknown") {
		settings["trust"] = p.Trust
	}
	if p.BcastCtl {
		settings["bcast_storm_ctl"] = "enabled"
	}
	if p.McastCtl {
		settings["mcast_storm_ctl"] = "enabled"
	}
	return settings
}

// SetSFPBaseline はベースラインのSFP Rx値をアラート判定基準に登録する。
func (e *Engine) SetSFPBaseline(base []*model.SwitchState) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.setSFPBaselineLocked(base)
}

func (e *Engine) setSFPBaselineLocked(base []*model.SwitchState) {
	e.baseline = map[string]float64{}
	for _, s := range base {
		for _, p := range s.Ports {
			if p.SFP != nil && p.Oper == "up" {
				e.baseline[itoa(int(s.ID))+":"+itoa(p.IfIndex)] = p.SFP.RxDBm
			}
		}
	}
}

func (e *Engine) CreateBaseline(name string) (int64, error) {
	e.operationMu.Lock()
	defer e.operationMu.Unlock()
	e.mu.Lock()
	id, err := e.saveBaselineLocked(name, "")
	e.mu.Unlock()
	if err == nil {
		e.OnEvent(baselineSavedEvent(name))
	}
	return id, err
}

func (e *Engine) saveBaselineLocked(name, phase string) (id int64, err error) {
	states := e.statesLocked()
	if phase != "" {
		id, err = e.st.SavePhaseBaseline(phase, e.profileID, name, states)
	} else {
		id, err = e.st.SaveBaseline(e.profileID, name, states)
	}
	if err == nil {
		e.setSFPBaselineLocked(states)
	}
	return id, err
}

func onoff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func macs(p *model.Port) string {
	var m []string
	for _, d := range p.Devices {
		m = append(m, d.MAC)
	}
	sort.Strings(m)
	return strings.Join(m, ",")
}

func poeOn(p *model.Port) string {
	if p.PoE != nil && p.PoE.PowerW > 0 {
		return "給電中"
	}
	return "なし"
}

// KnownIP は登録スイッチまたは観測済み機器のIPか（Ping対象の制限に使う）。
func (e *Engine) KnownIP(ip string) bool {
	for _, s := range e.States() {
		if s.IP == ip {
			return true
		}
		for _, p := range s.Ports {
			for _, d := range p.Devices {
				if d.IP == ip {
					return true
				}
			}
		}
	}
	return false
}
