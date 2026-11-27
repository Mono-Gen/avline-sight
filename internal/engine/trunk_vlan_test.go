package engine

import (
	"reflect"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// 実機（2026-11-27）の構成を写したもの: A の 0/25 は VLAN 1,11-14、B の 0/9 は VLAN 1,11 だけを運ぶ。
func trunkPair(aVlans, bVlans []int) []*model.SwitchState {
	a := &model.SwitchState{ID: 1, Name: "SW-A", SysName: "a", Ports: []*model.Port{{IfIndex: 25, Name: "0/25", Mode: "trunk", UntaggedVLANs: aVlans[:1], TaggedVLANs: aVlans[1:]}},
		Neighbors: []model.Neighbor{{LocalIfIndex: 25, SysName: "b", PortID: "0/9", IsNetgear: true}}}
	b := &model.SwitchState{ID: 2, Name: "SW-B", SysName: "b", Ports: []*model.Port{{IfIndex: 9, Name: "0/9", Mode: "trunk", UntaggedVLANs: bVlans[:1], TaggedVLANs: bVlans[1:]}},
		Neighbors: []model.Neighbor{{LocalIfIndex: 9, SysName: "a", PortID: "0/25", IsNetgear: true}}}
	return []*model.SwitchState{a, b}
}

// B にも VLAN 12 のポートがあるのに、リンクには VLAN 12 が流れていない → 途切れている（警告）。
// VLAN 13/14 は B に無い（片側だけで使う意図したVLAN）ので数えない。
func TestTrunkVlanMismatch(t *testing.T) {
	sw := trunkPair([]int{1, 11, 12, 13, 14}, []int{1, 11})
	sw[1].Ports = append(sw[1].Ports, &model.Port{IfIndex: 1, Name: "0/1", Mode: "general", UntaggedVLANs: []int{12}})
	ms := trunkVlanMismatches(sw)
	if len(ms) != 1 {
		t.Fatalf("1リンク1件のはず: %d件", len(ms))
	}
	m := ms[0]
	if m.PortA != "0/25" || m.PortB != "0/9" || !reflect.DeepEqual(m.OnlyA, []int{12}) || m.OnlyB != nil {
		t.Fatalf("got %+v", m)
	}
}

// 2026-11-27 実機の構成: VLAN 13/14 は A だけで使う設計で、B には無い → 警告しない。
func TestTrunkVlanOnlyOnOneSwitchIsIntended(t *testing.T) {
	if ms := trunkVlanMismatches(trunkPair([]int{1, 11, 12, 13, 14}, []int{1, 11, 12})); len(ms) != 0 {
		t.Fatalf("相手に無いVLANで警告した: %+v", ms)
	}
}

func TestTrunkVlanMatchNoFinding(t *testing.T) {
	if ms := trunkVlanMismatches(trunkPair([]int{1, 11}, []int{1, 11})); len(ms) != 0 {
		t.Fatalf("一致しているのに検出: %+v", ms)
	}
}

func TestTrunkVlanUnknownSkipped(t *testing.T) {
	sw := trunkPair([]int{1, 11}, []int{1})
	sw[1].Ports[0].UntaggedVLANs = nil // 片側が未取得
	if ms := trunkVlanMismatches(sw); len(ms) != 0 {
		t.Fatalf("未取得のリンクは判定しないはず: %+v", ms)
	}
}

// システム名が空のスイッチ同士（実機の既定）でも、シャーシID（本体MAC）で相手を特定できる。
func TestTrunkVlanMismatchWithoutSysName(t *testing.T) {
	sw := trunkPair([]int{1, 11, 12, 13, 14}, []int{1, 11, 12})
	// B にも VLAN 13/14 のポートがある（リンクに流れていないので途切れている）
	sw[1].Ports = append(sw[1].Ports, &model.Port{IfIndex: 1, Name: "0/1", UntaggedVLANs: []int{13}}, &model.Port{IfIndex: 2, Name: "0/2", UntaggedVLANs: []int{14}})
	sw[0].SysName, sw[1].SysName = "", ""
	sw[0].BaseMAC, sw[1].BaseMAC = "02:00:00:00:0A:01", "02:00:00:00:0B:02"
	sw[0].Neighbors[0].SysName, sw[0].Neighbors[0].ChassisID = "", "02:00:00:00:0b:02"
	sw[1].Neighbors[0].SysName, sw[1].Neighbors[0].ChassisID = "", "02:00:00:00:0A:01"
	ms := trunkVlanMismatches(sw)
	if len(ms) != 1 || !reflect.DeepEqual(ms[0].OnlyA, []int{13, 14}) || ms[0].PortB != "0/9" {
		t.Fatalf("got %+v", ms)
	}
}
