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
	set(mk(1, "A", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.0.2.11"}),
		mk(2, "B", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "non-querier", LastQuerier: "0.0.0.0"}))
	if got := codes(); got["QUERIER_CONFLICT"] || got["QUERIER_ABSENT"] {
		t.Fatalf("0.0.0.0 counted as a second querier or hid the real one: %v", got)
	}

	// 3) 実アドレスが2つ → 複数クエリア
	set(mk(1, "A", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.0.2.11"}),
		mk(2, "B", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", LastQuerier: "192.0.2.12"}))
	if got := codes(); !got["QUERIER_CONFLICT"] {
		t.Fatalf("two real queriers not detected: %v", got)
	}

	for _, c := range []struct {
		addr string
		want bool
	}{{"", false}, {"0.0.0.0", false}, {"::", false}, {"192.0.2.11", true}, {"fe80::1", true}} {
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
	poll("192.0.2.11")
	poll("0.0.0.0")
	if codes(t, st)["QUERIER_CHANGED"] != 0 {
		t.Fatal("0.0.0.0 reported as a querier change")
	}
	poll("192.0.2.12")
	if codes(t, st)["QUERIER_CHANGED"] != 1 {
		t.Fatal("a change from .1 to .2 through 0.0.0.0 was missed")
	}
}

// ファームウェアは同じ機種どうしで比べる。機種が違えば版が違っても不一致にしない。
func TestFirmwareMismatchComparesSameModelOnly(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name, mdl, fw string) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, SysName: name, Reachable: true, Model: mdl, FW: fw, LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: id == 1}}
	}
	audit := func(states ...*model.SwitchState) []Finding {
		e.mu.Lock()
		e.states = map[int64]*model.SwitchState{}
		e.order = nil
		for _, s := range states {
			e.states[s.ID] = s
			e.order = append(e.order, s.ID)
		}
		e.mu.Unlock()
		fs, _ := e.Audit()
		var out []Finding
		for _, f := range fs {
			if f.Code == "FW_MISMATCH" {
				out = append(out, f)
			}
		}
		return out
	}
	if got := audit(mk(1, "Core", "M4250-26G4F-PoE+", "13.0.7.6"), mk(2, "Stage", "M4350-24X4V", "14.0.1.2")); len(got) != 0 {
		t.Fatalf("different models compared: %+v", got)
	}
	got := audit(mk(1, "Core", "M4250-26G4F-PoE+", "13.0.7.6"), mk(2, "FOH", "M4250-26G4F-PoE+", "13.0.6.0"), mk(3, "Stage", "M4350-24X4V", "14.0.1.2"))
	if len(got) != 1 || got[0].Detail["versions"] != "M4250-26G4F-PoE+ — 13.0.6.0: FOH / 13.0.7.6: Core" {
		t.Fatalf("same-model mismatch: %+v", got)
	}
}

// 2026-11-27 実機: VLAN11 で A(.1) だけが querier、B(.254) は non-querier で A を最後のクエリアとして聞いている。
// A の LastQuerier には選出前に聞いた B の値が残っているが、選出は済んでいるので複数クエリアではない。
func TestSingleElectedQuerierWithStaleLastQuerier(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name string, v model.VlanIGMP) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, Reachable: true, FW: "13.0.7.6", LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: id == 1},
			IGMP:   model.IGMP{Vlans: []model.VlanIGMP{v}}}
	}
	e.mu.Lock()
	e.states = map[int64]*model.SwitchState{
		1: mk(1, "A", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "querier", QuerierAddr: "0.0.0.0", LastQuerier: "192.0.2.254"}),
		2: mk(2, "B", model.VlanIGMP{VLAN: 11, Snooping: true, QuerierOper: "non-querier", QuerierAddr: "0.0.0.0", LastQuerier: "192.0.2.1"}),
	}
	e.order = []int64{1, 2}
	e.mu.Unlock()
	fs, _ := e.Audit()
	for _, f := range fs {
		if f.Code == "QUERIER_CONFLICT" || f.Code == "QUERIER_ABSENT" {
			t.Fatalf("選出済みなのに %s: %s", f.Code, f.Title)
		}
	}
}

// 起動直後、非クエリア側のスイッチだけ IGMP を読み終えた状態では「クエリア不在」を出さない。
// 期待値は「読んでいないスイッチがクエリアかもしれない」ことから導く。全台そろえば従来どおり判定する。
func TestQuerierAbsentWaitsForAllSwitches(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name string, vlans []model.VlanIGMP) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, SysName: name, Reachable: true, FW: "13.0.7.6", LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: id == 1},
			IGMP:   model.IGMP{Vlans: vlans}}
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
	has := func(code string) bool {
		fs, _ := e.Audit()
		for _, f := range fs {
			if f.Code == code {
				return true
			}
		}
		return false
	}
	nonQ := []model.VlanIGMP{{VLAN: 11, Snooping: true, QuerierOper: "non-querier", LastQuerier: "0.0.0.0"}}

	// A（クエリア側）はまだ読んでいない
	set(mk(1, "A", nil), mk(2, "B", nonQ))
	if has("QUERIER_ABSENT") {
		t.Fatal("A を読む前にクエリア不在を出した")
	}
	// 全台読めて、本当にクエリアがいない → 出す
	set(mk(1, "A", []model.VlanIGMP{{VLAN: 11, Snooping: true, QuerierOper: "disabled", LastQuerier: "0.0.0.0"}}), mk(2, "B", nonQ))
	if !has("QUERIER_ABSENT") {
		t.Fatal("全台読めたのにクエリア不在を出さない")
	}
	// VLAN の行が無いスイッチ（空スライス＝読めた）は保留の理由にしない
	set(mk(1, "A", []model.VlanIGMP{}), mk(2, "B", nonQ))
	if !has("QUERIER_ABSENT") {
		t.Fatal("VLAN行の無いスイッチで判定が止まった")
	}
}

// スイッチ追加・再接続で一時的に IGMP が未収集になっても、出ている「クエリア不在」は消さない（確認済みの印を保つ）。
func TestQuerierAbsentKeptWhilePending(t *testing.T) {
	e, _ := newTestEngine(t)
	mk := func(id int64, name string, vlans []model.VlanIGMP) *model.SwitchState {
		return &model.SwitchState{ID: id, Name: name, SysName: name, Reachable: true, FW: "13.0.7.6", LastPoll: time.Now(),
			Config: model.SwitchConf{StpVersion: "rstp", StpPriority: 4096, IsRoot: id == 1},
			IGMP:   model.IGMP{Vlans: vlans}}
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
	active := func() bool {
		for _, a := range e.Alerts() {
			if a.Code == "QUERIER_ABSENT" && a.Active {
				return true
			}
		}
		return false
	}
	none := []model.VlanIGMP{{VLAN: 11, Snooping: true, QuerierOper: "disabled", LastQuerier: "0.0.0.0"}}
	set(mk(1, "A", none), mk(2, "B", none))
	e.evaluateCross(Settings{})
	if !active() {
		t.Fatal("前提: クエリア不在が出ていない")
	}
	// C を追加（IGMP 未収集）→ 不在は消えない
	set(mk(1, "A", none), mk(2, "B", none), mk(3, "C", nil))
	e.evaluateCross(Settings{})
	if !active() {
		t.Fatal("保留中に既存のクエリア不在が消えた")
	}
	// C を読めて C がクエリア → ここで消える
	set(mk(1, "A", none), mk(2, "B", none), mk(3, "C", []model.VlanIGMP{{VLAN: 11, Snooping: true, QuerierOper: "querier"}}))
	e.evaluateCross(Settings{})
	if active() {
		t.Fatal("クエリアが見つかったのに不在が残った")
	}
}
