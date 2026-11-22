package engine

import "github.com/Mono-Gen/avline-sight/internal/model"

func recommendationOnly(code string) bool {
	switch code {
	case "PAUSE_FRAMES", "EEE_ON_AV_PORT", "FLOWCTRL_ON_AV_PORT", "NDI4_NEEDS_FLOWCTRL", "DSCP_TRUST",
		"STORM_CTRL_UPLINK", "QSYS_MTU", "UPLINK_MTU_MISMATCH", "FW_MISMATCH", "STP_MSTP", "STP_PRIORITY_DEFAULT",
		"STP_EDGE_ROOT", "QUERIER_ON_EDGE", "SNOOP_MISMATCH", "IGMP_VERSION_MIX", "DAISY_CHAIN", "VLAN_CROWDED":
		return true
	}
	return false
}

type PortRecommendation struct {
	SwitchID    int64  `json:"switch_id"`
	Port        string `json:"port"`
	Code        string `json:"code"`
	Current     string `json:"current"`
	Recommended string `json:"recommended"`
	Source      string `json:"source,omitempty"`
	Reference   string `json:"reference,omitempty"`
}

func PortRecommendations(states []*model.SwitchState) []PortRecommendation {
	const proAVGuide = "NETGEAR ProAV Guide"
	rows := []PortRecommendation{}
	seen := map[PortRecommendation]bool{}
	add := func(s *model.SwitchState, p *model.Port, code, current, recommended, source, reference string) {
		row := PortRecommendation{s.ID, p.Name, code, current, recommended, source, reference}
		if !seen[row] {
			seen[row] = true
			rows = append(rows, row)
		}
	}
	byName := map[string]*model.SwitchState{}
	for _, s := range states {
		if s != nil && s.Reachable && s.SysName != "" {
			byName[s.SysName] = s
		}
	}
	for _, s := range states {
		if s == nil || !s.Reachable {
			continue
		}
		for _, p := range s.Ports {
			if p == nil || p.Excluded {
				continue
			}
			uplink := isUplink(s, p)
			av := isAVRole(p.AVRole) || (p.AVRole == "" && len(p.Devices) > 0 && !uplink)
			if av && p.EEE == "enable" {
				add(s, p, "EEE_ON_AV_PORT", p.EEE, "disable", "", "")
			}
			switch p.Pause {
			case "disabled":
				if p.AVRole == "ndi4" {
					add(s, p, "NDI4_NEEDS_FLOWCTRL", p.Pause, "enable", proAVGuide, "")
				}
			case "enabledXmit", "enabledRcv", "enabledXmitAndRcv":
				if av && p.AVRole != "ndi4" {
					add(s, p, "FLOWCTRL_ON_AV_PORT", p.Pause, "disable", "", "")
				}
			}
			if av && p.Trust != "" && p.Trust != "trustIpDscp" {
				add(s, p, "DSCP_TRUST", p.Trust, "trustIpDscp", "", "")
			}
			if uplink && p.BcastCtl {
				add(s, p, "BCAST_STORM_CTRL_UPLINK", "enable", "disable", proAVGuide, "pp. 14, 71")
			}
			if uplink && p.McastCtl {
				add(s, p, "MCAST_STORM_CTRL_UPLINK", "enable", "disable", proAVGuide, "pp. 14, 71")
			}
			if p.AVRole == "qsys" && p.MaxFrame > 1522 {
				add(s, p, "QSYS_MTU", itoa(p.MaxFrame), "1522", "", "")
			}
			if p.Duplex == "halfDuplex" {
				add(s, p, "DUPLEX", p.Duplex, "fullDuplex", "", "")
			}
		}
		for _, n := range s.Neighbors {
			peer := byName[n.SysName]
			if !n.IsNetgear || peer == nil || peer.ID == s.ID {
				continue
			}
			lp, rp := findPort(s, n.LocalPort), findPort(peer, n.PortID)
			if lp == nil || rp == nil || lp.Excluded || rp.Excluded || lp.MaxFrame <= 0 || rp.MaxFrame <= 0 || lp.MaxFrame == rp.MaxFrame {
				continue
			}
			add(s, lp, "UPLINK_MTU_MISMATCH", itoa(lp.MaxFrame), itoa(rp.MaxFrame), proAVGuide, "")
			add(peer, rp, "UPLINK_MTU_MISMATCH", itoa(rp.MaxFrame), itoa(lp.MaxFrame), proAVGuide, "")
		}
	}
	return rows
}
