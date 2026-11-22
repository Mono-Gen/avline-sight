package engine

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

// 期待値は .tasks の実装計画「機器の所在判定」「CSV形式」から導出（実装出力のコピーではない）。

func invStates() []*model.SwitchState {
	pw := func(w float64) *model.PoE { return &model.PoE{PowerW: w} }
	sw1 := &model.SwitchState{ID: 1, Name: "SW1", GroupTag: "FOH", Reachable: true, Ports: []*model.Port{
		{IfIndex: 5, Name: "1/0/5", SpeedMbps: 1000, PVID: 10, PoE: pw(12.5), Devices: []model.Device{
			{MAC: "00:11:22:33:44:55", Vendor: "Yamaha", VLAN: 10, Source: "fdb"}}},
		{IfIndex: 6, Name: "1/0/6", SpeedMbps: 100, PVID: 20, PoE: pw(30), Devices: []model.Device{ // 配下にハブ: PoEは機器ごとでない
			{MAC: "AA:00:00:00:00:01", VLAN: 20, Source: "fdb"},
			{MAC: "AA:00:00:00:00:02", VLAN: 20, Source: "fdb"}}},
		{IfIndex: 25, Name: "1/0/25", AVRole: "uplink", SpeedMbps: 10000, Devices: []model.Device{
			{MAC: "CC:00:00:00:00:01", Source: "fdb"}}},
	}}
	sw2 := &model.SwitchState{ID: 2, Name: "SW2", GroupTag: "STAGE", Reachable: true, Ports: []*model.Port{
		{IfIndex: 24, Name: "lag 1", Type: "lag", Devices: []model.Device{
			{MAC: "00:11:22:33:44:55", Source: "fdb"}, // SW1のアクセスポートの機器が経路上に見えている
			{MAC: "CC:00:00:00:00:01", LLDPName: "=cmd|evil", Source: "lldp"}}},
	}}
	return []*model.SwitchState{sw1, sw2}
}

func TestBuildInventoryLocation(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	seenAt := now.Add(-time.Hour)
	aliases := []store.Alias{
		{MatchType: "mac", MatchKey: "00:11:22:33:44:55", Label: "FOH-卓", Note: "メイン"},
		{MatchType: "mac", MatchKey: "DD:00:00:00:00:01", Label: "予備DSP"},
		{MatchType: "mac", MatchKey: "DD:00:00:00:00:02", Label: "一度も見ていない"},
		{MatchType: "port", MatchKey: "1:5", Label: "ポート名は対象外"},
	}
	rows := BuildInventory(invStates(), aliases, map[string]store.LastSeen{"DD:00:00:00:00:01": {TS: seenAt, IP: "10.0.0.99"}}, InventoryOptions{}, now)
	by := map[string]InventoryRow{}
	for _, r := range rows {
		if _, dup := by[r.MAC]; dup {
			t.Fatalf("MACが重複: %s", r.MAC)
		}
		by[r.MAC] = r
	}
	if len(rows) != 6 {
		t.Fatalf("行数 %d（期待6: 見えている4 + 未検出の登録2）", len(rows))
	}
	a := by["00:11:22:33:44:55"]
	if a.SwitchName != "SW1" || a.Port != "1/0/5" || a.ViaUplink || a.Label != "FOH-卓" || a.Note != "メイン" || a.PoEW == nil || *a.PoEW != 12.5 {
		t.Fatalf("アクセスポートの所在を優先していない: %+v", a)
	}
	c := by["CC:00:00:00:00:01"]
	if c.SwitchName != "SW2" || !c.ViaUplink || c.Source != "lldp" {
		t.Fatalf("アップリンクのみの機器はLLDPで名乗った所在を採るべき: %+v", c)
	}
	if by["AA:00:00:00:00:01"].PoEW != nil {
		t.Fatal("複数機器のポートでPoE値を機器に割り当てている")
	}
	if d := by["DD:00:00:00:00:01"]; d.Seen || !d.LastSeen.Equal(seenAt) || d.Port != "" || d.IP != "10.0.0.99" || !d.IPStale {
		t.Fatalf("未検出の登録機器: %+v", d)
	}
	if d := by["DD:00:00:00:00:02"]; !d.LastSeen.IsZero() {
		t.Fatalf("観測履歴のない登録機器に最終確認がある: %+v", d)
	}
	// 見えている機器が先、未検出は後
	if !rows[0].Seen || rows[len(rows)-1].Seen {
		t.Fatal("並び順: 見えている機器を先にすべき")
	}

	un := BuildInventory(invStates(), aliases, nil, InventoryOptions{Scope: "unnamed"}, now)
	if len(un) != 3 {
		t.Fatalf("名前なしは3件のはず: %d", len(un))
	}
	foh := BuildInventory(invStates(), aliases, nil, InventoryOptions{Group: "FOH"}, now)
	for _, r := range foh {
		if r.GroupTag != "FOH" {
			t.Fatalf("グループ絞込で他グループ・未検出が混入: %+v", r)
		}
	}
	if v := BuildInventory(invStates(), nil, nil, InventoryOptions{VLANs: []int{20}}, now); len(v) != 2 {
		t.Fatalf("VLAN絞込: %d件", len(v))
	}
	if q := BuildInventory(invStates(), aliases, nil, InventoryOptions{Query: "yamaha"}, now); len(q) != 1 {
		t.Fatalf("検索: %d件", len(q))
	}
}

func TestInventoryCSVRoundTrip(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	aliases := []store.Alias{{MatchType: "mac", MatchKey: "00:11:22:33:44:55", Label: "-FOH,卓\"A\"", Note: "改行\nあり"},
		{MatchType: "mac", MatchKey: "DD:00:00:00:00:02", Label: "未検出機器"}}
	rows := BuildInventory(invStates(), aliases, nil, InventoryOptions{}, now)
	var buf bytes.Buffer
	if err := WriteInventoryCSV(&buf, rows, time.UTC); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "\uFEFFMAC,名称,メモ,IP,メーカー,LLDP名,スイッチ,ポート,VLAN,PoE(W),リンク速度,最終確認,検出元,Ping\r\n") {
		t.Fatalf("見出し/BOM/CRLF: %q", out[:80])
	}
	if !strings.Contains(out, "'=cmd|evil") {
		t.Fatal("LLDP名の数式注入を無害化していない")
	}
	if !strings.Contains(out, "2026-01-02 03:04:05") || !strings.Contains(out, ",未検出,") || !strings.Contains(out, "lldp(経路上)") || !strings.Contains(out, ",1G,") {
		t.Fatalf("列の内容: %s", out)
	}
	// 書出したCSVをそのまま読み込むと、名称とメモが変わらずに戻る（往復）
	got, skipped, errs := ParseAliasCSV(out)
	if len(errs) != 0 {
		t.Fatalf("往復でエラー: %+v", errs)
	}
	if len(got) != 2 || skipped != 3 {
		t.Fatalf("名称あり2件・空欄3件のはず: %d %d", len(got), skipped)
	}
	m := map[string]AliasCSVRow{}
	for _, r := range got {
		m[r.MAC] = r
	}
	if r := m["00:11:22:33:44:55"]; r.Label != "-FOH,卓\"A\"" || r.Note != "改行\nあり" {
		t.Fatalf("往復で名称/メモが変化: %+v", r)
	}
}

func TestParseAliasCSV(t *testing.T) {
	// 見出しの列順が違っても列名で判定する。MAC表記の揺れを正規化する。
	csv := "名称,メモ,MAC\r\n" +
		"卓,,aa-bb-cc-dd-ee-01\r\n" +
		"DSP,ラック2,aabb.ccdd.ee02\r\n" +
		"PTZ,,AABBCCDDEE03\r\n" +
		",未記入,aa:bb:cc:dd:ee:04\r\n" +
		"\r\n" +
		"重複,,AA:BB:CC:DD:EE:01\r\n" +
		"不正,,zz:bb:cc:dd:ee:05\r\n" +
		"短い,,aa:bb:cc\r\n"
	rows, skipped, errs := ParseAliasCSV(csv)
	if len(rows) != 3 || skipped != 1 || len(errs) != 3 {
		t.Fatalf("rows=%+v skipped=%d errs=%+v", rows, skipped, errs)
	}
	if rows[0].MAC != "AA:BB:CC:DD:EE:01" || rows[1].MAC != "AA:BB:CC:DD:EE:02" || rows[2].MAC != "AA:BB:CC:DD:EE:03" || rows[1].Note != "ラック2" {
		t.Fatalf("正規化/列判定: %+v", rows)
	}
	if errs[0].Line != 7 || !strings.Contains(errs[0].Reason, "2行目") {
		t.Fatalf("重複の行番号: %+v", errs[0])
	}
	// 見出しなしは先頭3列
	rows, _, errs = ParseAliasCSV("00:11:22:33:44:55,卓,メモ\n")
	if len(errs) != 0 || len(rows) != 1 || rows[0].Label != "卓" || rows[0].Note != "メモ" {
		t.Fatalf("見出しなし: %+v %+v", rows, errs)
	}
	// 見出し風だがMAC/名称列が無い1行目はエラー行として報告する
	if _, _, errs = ParseAliasCSV("foo,bar\n"); len(errs) != 1 {
		t.Fatalf("不明な見出し: %+v", errs)
	}
	// 上限超過
	var b strings.Builder
	for i := 0; i <= MaxAliasCSVRows; i++ {
		b.WriteString("00:11:22:33:44:55,x\n")
	}
	if _, _, errs = ParseAliasCSV(b.String()); len(errs) == 0 {
		t.Fatal("上限超過を検出していない")
	}
}

func TestNormalizeMAC(t *testing.T) {
	for in, want := range map[string]string{"00-1a-2B-3c-4D-5e": "00:1A:2B:3C:4D:5E", " 001a.2b3c.4d5e ": "00:1A:2B:3C:4D:5E"} {
		if got, ok := NormalizeMAC(in); !ok || got != want {
			t.Fatalf("%q → %q", in, got)
		}
	}
	for _, bad := range []string{"", "00:1a:2b:3c:4d", "00:1a:2b:3c:4d:5e:6f", "g0:1a:2b:3c:4d:5e"} {
		if _, ok := NormalizeMAC(bad); ok {
			t.Fatalf("%q を受理した", bad)
		}
	}
}

func TestPingCell(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if got := pingCell(&PingMark{Status: "ok", RTTms: 2.6, At: at}, time.UTC); got != "応答 3ms (03:04:05)" {
		t.Fatal(got)
	}
	if got := pingCell(&PingMark{Status: "timeout", RTTms: -1, At: at}, time.UTC); got != "無応答 (03:04:05)" {
		t.Fatal(got)
	}
	if pingCell(nil, time.UTC) != "" {
		t.Fatal("未実施は空欄")
	}
}

// 監査指摘#4/#7/#5: 到達不能スイッチの一覧は過去の観測、IPは他の観測から補完、再生は記録時の名称を保つ
func TestInventoryAuditFixes(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	lastOK := now.Add(-10 * time.Minute)
	down := &model.SwitchState{ID: 3, Name: "SW3", Reachable: false, LastOK: lastOK, Ports: []*model.Port{
		{IfIndex: 1, Name: "1/0/1", Devices: []model.Device{{MAC: "EE:00:00:00:00:01", Label: "記録時の名前", Source: "fdb"}}}}}
	up := &model.SwitchState{ID: 4, Name: "SW4", Reachable: true, Ports: []*model.Port{
		{IfIndex: 2, Name: "1/0/2", Devices: []model.Device{{MAC: "EE:00:00:00:00:02", Source: "fdb"}}},
		{IfIndex: 25, Name: "1/0/25", AVRole: "uplink", Devices: []model.Device{{MAC: "EE:00:00:00:00:02", IP: "10.0.0.2", Source: "arp"}}}}}
	rows := BuildInventory([]*model.SwitchState{down, up}, nil, nil, InventoryOptions{}, now)
	by := map[string]InventoryRow{}
	for _, r := range rows {
		by[r.MAC] = r
	}
	if r := by["EE:00:00:00:00:01"]; r.Seen || !r.LastSeen.Equal(lastOK) || r.Port != "1/0/1" {
		t.Fatalf("到達不能スイッチの機器を見えている扱い: %+v", r)
	}
	if r := by["EE:00:00:00:00:02"]; r.Port != "1/0/2" || r.IP != "10.0.0.2" {
		t.Fatalf("所在はアクセスポート、IPはアップリンク側の観測から補完すべき: %+v", r)
	}
	al := []store.Alias{{MatchType: "mac", MatchKey: "EE:00:00:00:00:01", Label: "書出後に変えた名前", Note: "n"}}
	if r := BuildInventory([]*model.SwitchState{down}, al, nil, InventoryOptions{SnapshotLabels: true}, now)[0]; r.Label != "記録時の名前" || r.Note != "n" {
		t.Fatalf("再生で記録時の名称を上書き: %+v", r)
	}
	if r := BuildInventory([]*model.SwitchState{down}, al, nil, InventoryOptions{}, now)[0]; r.Label != "書出後に変えた名前" {
		t.Fatalf("ライブは登録名を優先: %+v", r)
	}
}

// 監査指摘#2/#1: 先頭が ' の名称も往復で変わらない。引用符の閉じ忘れはエラー（後続行を吸収しない）
func TestAliasCSVEdgeCases(t *testing.T) {
	for _, label := range []string{"'-FOH", "''x", "-FOH", "'abc", "=1+2"} {
		var buf bytes.Buffer
		WriteInventoryCSV(&buf, []InventoryRow{{MAC: "00:11:22:33:44:55", Label: label}}, time.UTC)
		rows, _, errs := ParseAliasCSV(buf.String())
		if len(errs) != 0 || len(rows) != 1 || rows[0].Label != label {
			t.Fatalf("%q が往復で変化: %+v %+v", label, rows, errs)
		}
	}
	rows, _, errs := ParseAliasCSV("MAC,名称,メモ\n00:11:22:33:44:01,A,\"閉じ忘れ\n00:11:22:33:44:02,B,\n")
	if len(errs) == 0 {
		t.Fatalf("引用符の閉じ忘れを検出していない: %+v", rows)
	}
}

// 監査2周目: IPは稼働中スイッチの観測を優先／削除済みの登録名を残さない／再生は空の名称も保つ／MAC列の引用符不正でpanicしない
func TestInventoryAuditRound2(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	down := &model.SwitchState{ID: 1, Name: "DOWN", Reachable: false, Ports: []*model.Port{
		{IfIndex: 1, Name: "1/0/1", Devices: []model.Device{{MAC: "EE:00:00:00:00:01", IP: "10.0.0.1", Label: "削除済みの名前", Source: "arp"}}}}}
	up := &model.SwitchState{ID: 2, Name: "UP", Reachable: true, Ports: []*model.Port{
		{IfIndex: 2, Name: "1/0/2", Devices: []model.Device{{MAC: "EE:00:00:00:00:02", Source: "fdb"}}},
		{IfIndex: 25, Name: "1/0/25", AVRole: "uplink", Devices: []model.Device{{MAC: "EE:00:00:00:00:02", IP: "10.0.0.22", Source: "arp"}}}}}
	down.Ports[0].Devices = append(down.Ports[0].Devices, model.Device{MAC: "EE:00:00:00:00:02", IP: "10.0.0.99", Source: "arp"})
	rows := BuildInventory([]*model.SwitchState{down, up}, nil, nil, InventoryOptions{}, now)
	by := map[string]InventoryRow{}
	for _, r := range rows {
		by[r.MAC] = r
	}
	if r := by["EE:00:00:00:00:02"]; r.IP != "10.0.0.22" {
		t.Fatalf("到達不能スイッチの古いIPを優先: %+v", r)
	}
	if r := by["EE:00:00:00:00:01"]; r.Label != "" {
		t.Fatalf("登録から消えた名称が残っている: %+v", r)
	}
	snap := BuildInventory([]*model.SwitchState{up}, []store.Alias{{MatchType: "mac", MatchKey: "EE:00:00:00:00:02", Label: "後日の名前"}}, nil, InventoryOptions{SnapshotLabels: true}, now)
	for _, r := range snap {
		if r.MAC == "EE:00:00:00:00:02" && r.Label != "" {
			t.Fatalf("記録時に名前なしだった機器へ後日の名前を適用: %+v", r)
		}
	}
	if _, _, errs := ParseAliasCSV("\"00:11:22:33:44:55,x\n"); len(errs) != 1 {
		t.Fatalf("MAC列の引用符不正: %+v", errs)
	}
}
