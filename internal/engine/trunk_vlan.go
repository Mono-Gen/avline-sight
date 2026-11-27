package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// trunkMismatch は、LLDPでつながった監視対象スイッチ同士のリンクで、両方のスイッチにあるのに片側にしか流れていないVLAN。
type trunkMismatch struct {
	A, B         *model.SwitchState
	PortA, PortB string
	OnlyA, OnlyB []int // A側だけ・B側だけに流れているVLAN
}

// trunkVlanMismatches はスイッチ間リンクの両端で流れるVLAN（tagged＋untagged）を比べる。
// 片側のVLAN一覧が取れていないリンクは判定しない（未取得を「VLANなし」と取り違えないため）。
// 相手が監視対象外のリンクも判定しない。
// 相手スイッチにそもそも無いVLAN（どのポートにも割り当てが無い）は、片側だけで使う意図したVLANとみなして数えない
// （2026-11-27 実機: VLAN 13/14 は .1 だけで使う設計。Auto-Trunk は全VLANを許可するため .1 側には流れて見える）。
func trunkVlanMismatches(live []*model.SwitchState) []trunkMismatch {
	ix := newSwitchIndex(live)
	var out []trunkMismatch
	for _, s := range live {
		for _, n := range s.Neighbors {
			peer := ix.peerOf(n)
			if !n.IsNetgear || peer == nil || peer.ID == s.ID || s.ID > peer.ID { // 1リンク1回（ID の小さい側から見る）
				continue
			}
			pa := portByIfIndex(s, n.LocalIfIndex)
			pb := peerPort(ix, peer, s, n.PortID)
			if pa == nil || pb == nil {
				continue
			}
			va, vb := carriedVLANs(pa), carriedVLANs(pb)
			if len(va) == 0 || len(vb) == 0 {
				continue
			}
			onlyA := withinVLANs(diffVLANs(va, vb), switchVLANs(peer))
			onlyB := withinVLANs(diffVLANs(vb, va), switchVLANs(s))
			if len(onlyA) == 0 && len(onlyB) == 0 {
				continue
			}
			out = append(out, trunkMismatch{A: s, B: peer, PortA: pa.Name, PortB: pb.Name, OnlyA: onlyA, OnlyB: onlyB})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].A.Name+out[i].PortA < out[j].A.Name+out[j].PortA
	})
	return out
}

func portByIfIndex(s *model.SwitchState, ix int) *model.Port {
	for i := range s.Ports {
		if s.Ports[i].IfIndex == ix {
			return s.Ports[i]
		}
	}
	return nil
}

// peerPort は相手スイッチ側のポートを探す。相手のLLDP隣接表で自分（from）を指す行のローカルポートを使う。
// 同じ相手と複数本つながっているときは、こちらのLLDPが報告したポートID（例 "0/9"）と名前が一致するものを選ぶ。
func peerPort(ix switchIndex, peer, from *model.SwitchState, remotePortID string) *model.Port {
	var cands []*model.Port
	for _, n := range peer.Neighbors {
		if !ix.pointsTo(n, from) {
			continue
		}
		if p := portByIfIndex(peer, n.LocalIfIndex); p != nil {
			cands = append(cands, p)
		}
	}
	if len(cands) == 1 {
		return cands[0]
	}
	for _, p := range cands {
		if remotePortID != "" && (p.Name == remotePortID || strings.HasSuffix(remotePortID, " "+p.Name)) {
			return p
		}
	}
	// 相手側のLLDPが無い・特定できないときは、ポートIDの名前で直接探す
	for i := range peer.Ports {
		if remotePortID != "" && peer.Ports[i].Name == remotePortID {
			return peer.Ports[i]
		}
	}
	return nil
}

// carriedVLANs はポートで流れているVLAN（tagged＋untagged）を昇順・重複なしで返す。
func carriedVLANs(p *model.Port) []int {
	seen := map[int]bool{}
	var vs []int
	for _, v := range append(append([]int{}, p.TaggedVLANs...), p.UntaggedVLANs...) {
		if !seen[v] {
			seen[v] = true
			vs = append(vs, v)
		}
	}
	sort.Ints(vs)
	return vs
}

func diffVLANs(a, b []int) []int {
	in := map[int]bool{}
	for _, v := range b {
		in[v] = true
	}
	var out []int
	for _, v := range a {
		if !in[v] {
			out = append(out, v)
		}
	}
	return out
}

func joinInts(vs []int) string {
	ss := make([]string, len(vs))
	for i, v := range vs {
		ss[i] = fmt.Sprint(v)
	}
	return strings.Join(ss, ",")
}

// switchIndex は LLDP の隣接情報から監視対象スイッチを引くための索引。
// システム名が未設定（空）のスイッチもあるため、名前・シャーシID（本体MAC）・管理IPのどれかで一致すればよい。
type switchIndex struct {
	byName, byMAC, byIP map[string]*model.SwitchState
}

func newSwitchIndex(live []*model.SwitchState) switchIndex {
	ix := switchIndex{map[string]*model.SwitchState{}, map[string]*model.SwitchState{}, map[string]*model.SwitchState{}}
	for _, s := range live {
		if s.SysName != "" {
			ix.byName[s.SysName] = s
		}
		if s.BaseMAC != "" {
			ix.byMAC[strings.ToUpper(s.BaseMAC)] = s
		}
		if s.IP != "" {
			ix.byIP[s.IP] = s
		}
	}
	return ix
}

// peerOf は隣接 n の相手スイッチ（監視対象でなければ nil）。
func (ix switchIndex) peerOf(n model.Neighbor) *model.SwitchState {
	if p := ix.byMAC[strings.ToUpper(n.ChassisID)]; p != nil {
		return p
	}
	if p := ix.byIP[n.MgmtIP]; n.MgmtIP != "" && p != nil {
		return p
	}
	if n.SysName != "" {
		return ix.byName[n.SysName]
	}
	return nil
}

// pointsTo は隣接 n が from を指しているか。
func (ix switchIndex) pointsTo(n model.Neighbor, from *model.SwitchState) bool {
	return ix.peerOf(n) == from
}

// switchVLANs はスイッチのどこかのポートに割り当てのあるVLAN。
func switchVLANs(s *model.SwitchState) map[int]bool {
	in := map[int]bool{}
	for _, p := range s.Ports {
		for _, v := range carriedVLANs(p) {
			in[v] = true
		}
	}
	return in
}

// withinVLANs は vs のうち in にあるものだけを返す。
func withinVLANs(vs []int, in map[int]bool) []int {
	var out []int
	for _, v := range vs {
		if in[v] {
			out = append(out, v)
		}
	}
	return out
}

// vlanNameMismatch はスイッチ間リンクで両端に流れている同じ番号の VLAN が、スイッチごとに違う名前を持つもの。
type vlanNameMismatch struct {
	A, B         *model.SwitchState
	PortA        string
	VLAN         int
	NameA, NameB string
}

// vlanNameMismatches は、LLDPでつながった監視対象スイッチ同士のリンクで両端に流れている VLAN の名前を比べる。
// 動作には影響しないが、名前が違うと同じ VLAN か取り違えやすい（2026-11-27 実機: VLAN11 が "Dante-Pri" と "dante"）。
// 名前が取れていない（空）側があれば比べない。大文字小文字と前後の空白だけの違いは同じ名前とみなす。
// 同じスイッチの組で同じ VLAN は1回だけ数える（LAG など複数本のリンクで重複させないため）。
func vlanNameMismatches(live []*model.SwitchState) []vlanNameMismatch {
	ix := newSwitchIndex(live)
	seen := map[string]bool{}
	var out []vlanNameMismatch
	for _, s := range live {
		for _, n := range s.Neighbors {
			peer := ix.peerOf(n)
			if !n.IsNetgear || peer == nil || peer.ID == s.ID || s.ID > peer.ID {
				continue
			}
			pa := portByIfIndex(s, n.LocalIfIndex)
			pb := peerPort(ix, peer, s, n.PortID)
			if pa == nil || pb == nil {
				continue
			}
			na, nb := vlanNameMap(s), vlanNameMap(peer)
			inB := map[int]bool{}
			for _, v := range carriedVLANs(pb) {
				inB[v] = true
			}
			for _, v := range carriedVLANs(pa) {
				key := fmt.Sprintf("%d|%d|%d", s.ID, peer.ID, v)
				a, b := strings.TrimSpace(na[v]), strings.TrimSpace(nb[v])
				if !inB[v] || seen[key] || a == "" || b == "" || strings.EqualFold(a, b) {
					continue
				}
				seen[key] = true
				out = append(out, vlanNameMismatch{A: s, B: peer, PortA: pa.Name, VLAN: v, NameA: a, NameB: b})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].A.Name+out[i].B.Name != out[j].A.Name+out[j].B.Name {
			return out[i].A.Name+out[i].B.Name < out[j].A.Name+out[j].B.Name
		}
		return out[i].VLAN < out[j].VLAN
	})
	return out
}

// vlanNameMap はスイッチの VLAN 番号→名前（IGMP の VLAN 一覧に入っている名前）。
func vlanNameMap(s *model.SwitchState) map[int]string {
	m := map[int]string{}
	for _, v := range s.IGMP.Vlans {
		m[v.VLAN] = v.Name
	}
	return m
}
