package sim

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
)

// 実際のgosnmpクライアントで仮想エージェントに GET/WALK/BULKWALK できることを確認する結合テスト。
func TestAgentRoundTrip(t *testing.T) {
	lab := NewLab(3, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := lab.Start(ctx, Config{BaseSNMPPort: 26100}); err != nil {
		t.Fatal(err)
	}
	g := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: 26101, Community: Community, Version: gosnmp.Version2c, Timeout: 2 * time.Second, MaxRepetitions: 20}
	if err := g.Connect(); err != nil {
		t.Fatal(err)
	}
	defer g.Conn.Close()

	r, err := g.Get([]string{mib.SysName, mib.SysUpTime, "1.3.6.1.2.1.1.99.0"})
	if err != nil {
		t.Fatal(err)
	}
	if string(r.Variables[0].Value.([]byte)) != "AV-SW-02" {
		t.Errorf("sysName=%v", r.Variables[0].Value)
	}
	if r.Variables[2].Type != gosnmp.NoSuchObject {
		t.Errorf("存在しないOIDはNoSuchObjectであるべき: %v", r.Variables[2].Type)
	}

	pdus, err := g.BulkWalkAll(mib.IfName)
	if err != nil {
		t.Fatal(err)
	}
	if len(pdus) != len(lab.Switches[1].Ports) {
		t.Errorf("ifName件数=%d want %d", len(pdus), len(lab.Switches[1].Ports))
	}
	// 64bitカウンタは時間経過で増加する
	a, _ := g.Get([]string{mib.IfHCInOctets + ".1"})
	time.Sleep(300 * time.Millisecond)
	b, _ := g.Get([]string{mib.IfHCInOctets + ".1"})
	if gosnmp.ToBigInt(b.Variables[0].Value).Cmp(gosnmp.ToBigInt(a.Variables[0].Value)) <= 0 {
		t.Error("ifHCInOctetsが増加していない")
	}
	// LLDP: エッジのアップリンクからコア名が見える
	lp, err := g.BulkWalkAll(mib.LldpRemSysName)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range lp {
		if string(p.Value.([]byte)) == "AV-SW-01" {
			found = true
		}
	}
	if !found {
		t.Error("LLDPでコアスイッチが見えない")
	}
	// コミュニティ不一致は無応答（タイムアウト）
	bad := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: 26101, Community: "wrong", Version: gosnmp.Version2c, Timeout: 300 * time.Millisecond, Retries: 0}
	bad.Connect()
	if _, err := bad.Get([]string{mib.SysName}); err == nil {
		t.Error("不正コミュニティに応答してしまった")
	}
	bad.Conn.Close()
}

func TestInjectAll(t *testing.T) {
	lab := NewLab(2, 1)
	for _, sc := range Scenarios {
		if err := lab.Inject(1, sc.ID); err != nil {
			t.Errorf("%s: %v", sc.ID, err)
		}
	}
	if err := lab.Inject(5, "chassis"); err == nil {
		t.Error("範囲外スイッチでエラーにならない")
	}
	lab.Reset()
	for _, p := range lab.Switches[1].Ports {
		if p.EEE || p.FCSRate != 0 {
			t.Fatal("Resetで障害が残っている")
		}
	}
	// ツリー生成がpanicしない＆ソート済み
	es := lab.Switches[0].build(lab)
	if len(es) < 100 {
		t.Fatalf("エントリ数が少なすぎる: %d", len(es))
	}
	out := lab.showOutput(lab.Switches[0], "show igmpsnooping querier")
	if !strings.Contains(out, "Querier") {
		t.Error("CLI応答が不正")
	}
}
