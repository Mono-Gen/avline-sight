package engine

import (
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// M4250 は設定上の既定値として 0.0.0.0 を返す（set igmp querier N address 0.0.0.0）。
// 期待値は「0.0.0.0 はクエリアがいる証拠にならない」という規則から導く。
func TestQuerierAddressZeroIsNotAQuerier(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name string, v model.VlanIGMP) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, SysName: name, Reachable: true, FW: "13.0.7.6", LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: id == 1},
			IGMP:   model.IGMP{Vlans: []model.VlanIGMP{v}}}
	}
	codes := func() map[string]bool {
		fs, _ := e.Audit()
		got := map[string]bool{}
		for _, f := range fs {
			got[f.Code] = true
		}
		return got
	}
	set := func(states ...*model.SwitchState) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.states = map[int64]*model.SwitchState{}
		e.order = nil
		for _, s := range states {
			e.states[s.ID] = s
			e.order = append(e.order, s.ID)
		}
	}

	// 1) Snooping有効でクエリアなし、最後のクエリアも 0.0.0.0 だけ → 不在と判定する
	set(mk(1, "A", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "disabled", LastQuerier: "0.0.0.0"}),
		mk(2, "B", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "disabled", LastQuerier: "0.0.0.0"}))
	if got := codes(); !got["QUERIER_ABSENT"] {
		t.Fatalf("0.0.0.0 hid a missing querier: %v", got)
	}

	// 2) 片方が実アドレス、もう片方が 0.0.0.0 → 複数クエリアではない
	set(mk(1, "A", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.168.11.1"}),
		mk(2, "B", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "non-querier", LastQuerier: "0.0.0.0"}))
	if got := codes(); got["QUERIER_CONFLICT"] || got["QUERIER_ABSENT"] {
		t.Fatalf("0.0.0.0 counted as a second querier or hid the real one: %v", got)
	}

	// 3) 実アドレスが2つ → 複数クエリア
	set(mk(1, "A", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.168.11.1"}),
		mk(2, "B", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.168.11.2"}))
	if got := codes(); !got["QUERIER_CONFLICT"] {
		t.Fatalf("two real queriers not detected: %v", got)
	}

	for _, c := range []struct {
		addr string
		want bool
	}{{"", false}, {"0.0.0.0", false}, {"::", false}, {"192.168.11.1", true}, {"fe80::1", true}} {
		if got := realQuerier(c.addr); got != c.want {
			t.Errorf("realQuerier(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

// クエリアが落ちて（0.0.0.0 を挟んで）別のクエリアに替わったときも QUERIER_CHANGED を出す。
// QUERIER_CHANGED は一度きりの出来事（発生中のアラートにはならない）なので、記録されたイベントで確かめる。
func TestQuerierChangeThroughZero(t *testing.T) {
	e, st := newTestEngine(t)
	poll := func(last string) {
		e.mu.Lock()
		e.states = map[int64]*model.SwitchState{1: {ID: 1, Name: "A", SysName: "A", Reachable: true, FW: "13.0.7.6", LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: true},
			IGMP:   model.IGMP{SnoopingKnown: true, SnoopingOn: true, Vlans: []model.VlanIGMP{{VLAN: 11, Snooping: true, LastQuerier: last}}}}}
		e.order = []int64{1}
		e.mu.Unlock()
		e.evaluateCross(DefaultSettings())
	}
	poll("192.168.11.1")
	poll("0.0.0.0")
	if codes(t, st)["QUERIER_CHANGED"] != 0 {
		t.Fatal("0.0.0.0 reported as a querier change")
	}
	poll("192.168.11.2")
	if codes(t, st)["QUERIER_CHANGED"] != 1 {
		t.Fatal("a change from .1 to .2 through 0.0.0.0 was missed")
	}
}
