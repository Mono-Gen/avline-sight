package engine

import (
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// 期待値は規則から導く: ほぼ全機器が参加するグループ（224.0.0.x・PTP）は「ストリームを受けている」根拠にしない。
func TestBaseGroup(t *testing.T) {
	for _, c := range []struct {
		group     string
		base, mac bool
	}{
		{"01:00:5E:00:00:FB", true, true},   // mDNS 224.0.0.251（M4250はMACで返す）
		{"01:00:5e:00:01:81", true, true},   // PTP 224.0.1.129（小文字でも同じ）
		{"224.0.0.251", true, false},        // IP書式
		{"224.0.1.130", true, false},        // PTP
		{"01:00:5E:45:00:01", false, true},  // 例: 239.69.0.1 のような実ストリーム
		{"239.69.0.1", false, false},
		{"", false, false},
	} {
		if base, mac := baseGroup(c.group); base != c.base || mac != c.mac {
			t.Errorf("baseGroup(%q) = %v,%v, want %v,%v", c.group, base, mac, c.base, c.mac)
		}
	}
}

func floodSwitch(snooping bool, groups []model.McastGroup, sourcePps, txPps float64) *model.SwitchState {
	port := func(ix int, name string, rx, tx float64) *model.Port {
		return &model.Port{IfIndex: ix, Name: name, Oper: "up", PVID: 11, Type: "rj45", RxMcPps: rx, TxMcPps: tx}
	}
	return &model.SwitchState{ID: 1, Name: "A", Reachable: true,
		IGMP: model.IGMP{SnoopingOn: true, SnoopingKnown: true, Vlans: []model.VlanIGMP{{VLAN: 11, Snooping: snooping}}, Groups: groups},
		Ports: []*model.Port{port(1, "0/1", sourcePps, 0), port(3, "0/3", 0, txPps), port(5, "0/5", 0, txPps), port(7, "0/7", 0, txPps)}}
}

func TestFloodedPorts(t *testing.T) {
	base := []model.McastGroup{
		{VLAN: 11, Group: "01:00:5E:00:01:81", Ports: []string{"0/1", "0/3", "0/5", "0/7"}}, // PTP
		{VLAN: 11, Group: "01:00:5E:00:00:FB", Ports: []string{"0/1", "0/3", "0/5", "0/7"}}, // mDNS
	}
	// 1) 参加は PTP・mDNS だけの3ポートへ、流入（3000pps）と同じ量が出ている → 3ポートがフラッディング
	//    （以前の判定では PTP/mDNS への参加で全ポートが対象外になり、0 だった）
	if got := floodedPorts(floodSwitch(true, base, 3000, 3000)); got != 3 {
		t.Errorf("flooding to ports joined only to PTP/mDNS: %d, want 3", got)
	}
	// 2) 3ポートとも実ストリームに参加している → 正規配信なのでフラッディングではない
	joined := append(base, model.McastGroup{VLAN: 11, Group: "01:00:5E:45:00:01", Ports: []string{"0/3", "0/5", "0/7"}})
	if got := floodedPorts(floodSwitch(true, joined, 3000, 3000)); got != 0 {
		t.Errorf("regular delivery to joined ports counted as flooding: %d", got)
	}
	// 3) Snooping無効のVLANは全ポートへ配るのが正しい動作 → 対象外
	if got := floodedPorts(floodSwitch(false, base, 3000, 3000)); got != 0 {
		t.Errorf("a VLAN without snooping counted as flooding: %d", got)
	}
	// 4) 流入が floodMinPps 未満（告知程度）→ 判定しない
	if got := floodedPorts(floodSwitch(true, base, floodMinPps-1, floodMinPps-1)); got != 0 {
		t.Errorf("tiny multicast counted as flooding: %d", got)
	}
	// 5) 出ている量が流入の半分未満 → フラッディングではない
	if got := floodedPorts(floodSwitch(true, base, 3000, 1000)); got != 0 {
		t.Errorf("a small share of the source counted as flooding: %d", got)
	}
	// 6) 全体のSnoopingがOFF（取得できている）→ ここでは数えない（SNOOP_GLOBAL_OFF で知らせる）
	off := floodSwitch(true, base, 3000, 3000)
	off.IGMP.SnoopingOn = false
	if got := floodedPorts(off); got != 0 {
		t.Errorf("flooding counted while snooping is off for the whole switch: %d", got)
	}
	// 7) ストリームがトランク（PVID 1・タグ付き、LLDPでNETGEAR隣接）から入り、アクセスポートへ複製 → 検知する
	trunk := floodSwitch(true, base, 0, 3000)
	trunk.Ports = append(trunk.Ports, &model.Port{IfIndex: 25, Name: "0/25", Oper: "up", PVID: 1, Type: "rj45", RxMcPps: 3000})
	trunk.Neighbors = []model.Neighbor{{LocalIfIndex: 25, IsNetgear: true}}
	if got := floodedPorts(trunk); got != 3 {
		t.Errorf("flooding from a trunk source: %d, want 3", got)
	}
	// 8) MAC書式の「基本グループ」(01:00:5E:00:00:05) に一部のポート（4本中 0/3・0/5 の2本＝過半でない）だけが参加
	//    → 実ストリーム（例 239.0.0.5）の正規受信として扱う。残る未参加は 0/7 の1本なので検知しない（誤報しない）
	alias := append([]model.McastGroup{}, base...)
	alias = append(alias, model.McastGroup{VLAN: 11, Group: "01:00:5E:00:00:05", Ports: []string{"0/3", "0/5"}})
	if got := floodedPorts(floodSwitch(true, alias, 3000, 3000)); got != 0 {
		t.Errorf("a stream that aliases a base MAC, joined by some ports, counted as flooding: %d", got)
	}
	// 9) 送信元自身は「参加していないポート」に数えない: 送信元＋未参加3本（2本へ複製、1本は複製なし）→ 2/3 で検知
	src := floodSwitch(true, base, 3000, 3000)
	src.Ports[3].TxMcPps = 0 // 0/7 には出ていない
	if got := floodedPorts(src); got != 2 {
		t.Errorf("source port counted as a candidate: %d, want 2", got)
	}
	// 10) MAC書式の別名グループの参加者にトランク(0/25)が含まれても、分子はアクセスポートだけで数える
	//     （アクセス4本中2本の参加＝過半でない → 実ストリームの正規受信として扱い、誤報しない）
	withTrunk := floodSwitch(true, append(append([]model.McastGroup{}, base...),
		model.McastGroup{VLAN: 11, Group: "01:00:5E:00:00:05", Ports: []string{"0/3", "0/5", "0/25"}}), 3000, 3000)
	withTrunk.Ports = append(withTrunk.Ports, &model.Port{IfIndex: 25, Name: "0/25", Oper: "up", PVID: 1, Type: "rj45"})
	withTrunk.Neighbors = []model.Neighbor{{LocalIfIndex: 25, IsNetgear: true}}
	if got := floodedPorts(withTrunk); got != 0 {
		t.Errorf("trunk counted as an access member of an aliased group: %d", got)
	}
	// 11) トランクが別VLANの大流量(20,000pps)を運んでいても、このVLANのローカル送信元(3,000pps)からのフラッディングは検知する
	busyTrunk := floodSwitch(true, base, 3000, 3000)
	busyTrunk.Ports = append(busyTrunk.Ports, &model.Port{IfIndex: 25, Name: "0/25", Oper: "up", PVID: 1, Type: "rj45", RxMcPps: 20000})
	busyTrunk.Neighbors = []model.Neighbor{{LocalIfIndex: 25, IsNetgear: true}}
	if got := floodedPorts(busyTrunk); got != 3 {
		t.Errorf("local flooding missed because a trunk carries more traffic: %d, want 3", got)
	}
	// 12) 参加していない送信機（各2,000pps送出）がフラッディング(3,000pps)を受けている → 送信機でも数える
	senders := floodSwitch(true, base, 3000, 3000)
	for _, p := range senders.Ports[1:] {
		p.RxMcPps = 2000
	}
	if got := floodedPorts(senders); got != 3 {
		t.Errorf("non-joined senders that receive the flood were not counted: %d, want 3", got)
	}
}

func TestSnoopGlobalOff(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(known, on bool) *model.SwitchState {
		return &model.SwitchState{ID: 1, Name: "A", SysName: "A", Reachable: true, FW: "13.0.7.6", LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: true},
			IGMP: model.IGMP{SnoopingKnown: known, SnoopingOn: on,
				Vlans: []model.VlanIGMP{{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.0.2.11"}, {VLAN: 13}}}}
	}
	audit := func(s *model.SwitchState) map[string]Finding {
		e.mu.Lock()
		e.states = map[int64]*model.SwitchState{1: s}
		e.order = []int64{1}
		e.mu.Unlock()
		fs, _ := e.Audit()
		got := map[string]Finding{}
		for _, f := range fs {
			got[f.Code] = f
		}
		return got
	}
	got := audit(mk(true, false))
	f, ok := got["SNOOP_GLOBAL_OFF"]
	if !ok || f.Severity != model.SevCritical || f.Detail["vlans"] != "11" {
		t.Fatalf("switch-wide snooping off not reported for VLAN 11: %+v", got)
	}
	if _, ok := audit(mk(true, true))["SNOOP_GLOBAL_OFF"]; ok {
		t.Fatal("reported while snooping is on")
	}
	// 取得できなかったときは OFF と見なさない（誤報しない）
	if _, ok := audit(mk(false, false))["SNOOP_GLOBAL_OFF"]; ok {
		t.Fatal("reported although the switch-wide setting could not be read")
	}
}
