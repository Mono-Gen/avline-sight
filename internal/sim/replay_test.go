package sim

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

// 期待値はダンプ書式の仕様（snmpdump の出力形式）から決めている。
func TestInferValue(t *testing.T) {
	cases := []struct {
		oid, val string
		typ      gosnmp.Asn1BER
	}{
		{".1.3.6.1.2.1.1.5.0", `|""`, gosnmp.OctetString},
		{".1.3.6.1.2.1.31.1.1.1.1.1", `302f31|"0/1"`, gosnmp.OctetString},
		{".1.3.6.1.2.1.1.2.0", ".1.3.6.1.4.1.4526.100.4.71", gosnmp.ObjectIdentifier},
		{".1.3.6.1.2.1.4.20.1.1.10.0.0.1", "10.0.0.1", gosnmp.IPAddress},
		{".1.3.6.1.2.1.1.3.0", "1825683", gosnmp.TimeTicks},
		{".1.3.6.1.2.1.2.2.1.9.3", "100", gosnmp.TimeTicks},
		{".1.3.6.1.2.1.2.2.1.10.1", "69882129", gosnmp.Counter32},
		{".1.3.6.1.2.1.31.1.1.1.6.1", "12345678901", gosnmp.Counter64},
		{".1.3.6.1.2.1.31.1.1.1.15.1", "1000", gosnmp.Gauge32},
		{".1.3.6.1.2.1.2.2.1.8.1", "1", gosnmp.Integer},
		// ifIndex 1 の ifSpeed の接頭辞が ifInOctets(.10) と紛れないこと（"…1.1" と "…1.10" の区別）
		{".1.3.6.1.2.1.2.2.1.1.10", "10", gosnmp.Integer},
		// 符号付き32bitに入らない数値は値を保つため Counter64
		{".1.3.6.1.4.1.4526.10.1.2.15.10.1.8.0", "3000000000", gosnmp.Counter64},
		// dot3PauseTable の .2 は設定値、.5 は64bitカウンタ
		{".1.3.6.1.2.1.10.7.10.1.2.1", "4", gosnmp.Integer},
		{".1.3.6.1.2.1.10.7.10.1.5.1", "5000000000", gosnmp.Counter64},
		{".1.3.6.1.2.1.105.1.1.1.13.1.1", "2", gosnmp.Counter32},
		// 300 は IPv4 として無効なので IpAddress にしない
		{".1.3.6.1.2.1.1.9.1.3.1", "300.1.1.1", gosnmp.OctetString},
	}
	for _, c := range cases {
		typ, v := inferValue(c.oid, c.val)
		if typ != c.typ {
			t.Errorf("%s=%s: 型 %v、期待 %v", c.oid, c.val, typ, c.typ)
		}
		if c.val == `302f31|"0/1"` && string(v.([]byte)) != "0/1" {
			t.Errorf("16進の復号が違う: %q", v)
		}
		if c.val == `|""` && len(v.([]byte)) != 0 {
			t.Errorf("空文字列が空にならない: %q", v)
		}
	}
}

func TestReplayServesDump(t *testing.T) {
	dir := t.TempDir()
	// 2ファイルに分かれた1台分＋別の1台。重複OIDは先に読んだファイル（名前順）を採る。
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("snmp_10.0.0.1.txt", ".1.3.6.1.2.1.31.1.1.1.1.1 = 302f31|\"0/1\"\n.1.3.6.1.2.1.31.1.1.1.1.2 = 302f32|\"0/2\"\n")
	// 見出し行は飛ばす。先頭の "." の有無が違う同じOIDは1件にまとまり、先に出た値を採る
	write("snmp_std_10.0.0.1.txt", "# target = 10.0.0.1\n.1.3.6.1.2.1.1.3.0 = 1000\n1.3.6.1.2.1.1.3.0 = 999\n.1.3.6.1.2.1.1.5.0 = 7377|\"sw\"\n")
	write("snmp_10.0.0.2.txt", ".1.3.6.1.2.1.1.3.0 = 5\n")
	write("README.txt", "対象外のファイル\n")

	rp, err := LoadReplay(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.Agents) != 2 || rp.Agents[0].SourceIP != "10.0.0.1" || rp.Agents[1].SourceIP != "10.0.0.2" {
		t.Fatalf("台の分け方が違う: %+v", rp.Agents)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const base = 26150
	if err := rp.Start(ctx, base); err != nil {
		t.Fatal(err)
	}
	g := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: base, Community: Community, Version: gosnmp.Version2c, Timeout: 2 * time.Second, Retries: 1}
	if err := g.Connect(); err != nil {
		t.Fatal(err)
	}
	defer g.Conn.Close()

	// GETNEXT で ifName を順に辿れる
	var names []string
	if err := g.Walk(".1.3.6.1.2.1.31.1.1.1.1", func(p gosnmp.SnmpPDU) error {
		names = append(names, string(p.Value.([]byte)))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "0/1" || names[1] != "0/2" {
		t.Fatalf("ifName: %v", names)
	}
	// GETBULK でも同じ
	bulk, err := g.GetBulk([]string{".1.3.6.1.2.1.31.1.1.1.1"}, 0, 5)
	if err != nil || len(bulk.Variables) == 0 || string(bulk.Variables[0].Value.([]byte)) != "0/1" {
		t.Fatalf("GETBULK: %+v %v", bulk, err)
	}
	// sysUpTime は TimeTicks で、ダンプ値以上（巻き戻らない）
	r, err := g.Get([]string{".1.3.6.1.2.1.1.3.0", ".1.3.6.1.2.1.1.5.0"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Variables[0].Type != gosnmp.TimeTicks || gosnmp.ToBigInt(r.Variables[0].Value).Uint64() < 1000 {
		t.Fatalf("sysUpTime: %+v", r.Variables[0])
	}
	if string(r.Variables[1].Value.([]byte)) != "sw" {
		t.Fatalf("別ファイルの値が混ざっていない: %+v", r.Variables[1])
	}
}
