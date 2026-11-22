package engine

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

// 接続機器一覧（台帳）とその CSV 書出・機器名 CSV の解釈。
// 責任: スイッチ状態から MAC ごとに所在を1か所へ絞った一覧を作ること、CSV の入出力形式。
// 対象外: 永続化（store）、HTTP（api）、文字コード判定（ブラウザ側で UTF-8 / Shift_JIS をデコードして渡す）。

// InventoryRow は接続機器一覧の1行（MAC単位）。
type InventoryRow struct {
	MAC        string    `json:"mac"`
	Label      string    `json:"label"`
	Note       string    `json:"note"`
	IP         string    `json:"ip"`
	Vendor     string    `json:"vendor"`
	LLDPName   string    `json:"lldp_name"`
	SwitchID   int64     `json:"switch_id,omitempty"`
	SwitchName string    `json:"switch_name"`
	GroupTag   string    `json:"group_tag"`
	Port       string    `json:"port"`
	IfIndex    int       `json:"if_index,omitempty"`
	VLAN       int       `json:"vlan"`
	PoEW       *float64  `json:"poe_w,omitempty"`
	SpeedMbps  int       `json:"speed_mbps"`
	Seen       bool      `json:"seen"`                // 現在（または再生時刻に）見えているか
	LastSeen   time.Time `json:"last_seen,omitempty"` // 見えていない登録済み機器の最終観測（不明ならゼロ）
	Source     string    `json:"source"`
	ViaUplink  bool      `json:"via_uplink"` // スイッチ間リンクでしか見えていない（所在は経路上の推定）
}

// InventoryOptions は一覧の絞込条件。
type InventoryOptions struct {
	Scope string // "all"（既定）/ "unnamed"
	Group string // 担当パート（スイッチの group_tag）。空なら全体
	Query string // MAC / IP / 名称 / メーカー / LLDP名 / スイッチ / ポート の部分一致
	VLANs []int  // 担当パート（表示プリセット）のVLAN。空なら全VLAN
}

// BuildInventory は状態から MAC ごとに1行の一覧を作る。
// at は「見えている」機器の最終確認時刻（ライブは現在、再生はスナップショット時刻）。
// lastSeen は見えていない登録済み機器の最終観測時刻（device_bindings 由来、無ければ欠落）。
func BuildInventory(states []*model.SwitchState, aliases []store.Alias, lastSeen map[string]time.Time, opt InventoryOptions, at time.Time) []InventoryRow {
	type cand struct {
		row    InventoryRow
		uplink bool
		lldp   bool
	}
	best := map[string]cand{}
	better := func(a, b cand) bool { // a が b より所在として確からしいか
		if a.uplink != b.uplink {
			return !a.uplink
		}
		if a.lldp != b.lldp {
			return a.lldp
		}
		return false // 同条件なら先に見つけた方（スイッチ順・ポート順）を保つ
	}
	for _, s := range states {
		if s == nil {
			continue
		}
		for _, p := range s.Ports {
			up := isUplink(s, p)
			for _, d := range p.Devices {
				r := InventoryRow{MAC: d.MAC, Label: d.Label, IP: d.IP, Vendor: d.Vendor, LLDPName: d.LLDPName,
					SwitchID: s.ID, SwitchName: s.Name, GroupTag: s.GroupTag, Port: p.Name, IfIndex: p.IfIndex,
					VLAN: d.VLAN, SpeedMbps: p.SpeedMbps, Seen: true, LastSeen: at, Source: d.Source, ViaUplink: up}
				if r.VLAN == 0 {
					r.VLAN = p.PVID
				}
				// 1ポートに複数機器（配下にハブ）なら、ポートのPoE値は機器ごとの値ではないため出さない
				if p.PoE != nil && len(p.Devices) == 1 {
					w := p.PoE.PowerW
					r.PoEW = &w
				}
				c := cand{row: r, uplink: up, lldp: d.Source == "lldp"}
				if old, ok := best[d.MAC]; !ok || better(c, old) {
					best[d.MAC] = c
				}
			}
		}
	}
	rows := make([]InventoryRow, 0, len(best)+len(aliases))
	for _, c := range best {
		rows = append(rows, c.row)
	}
	// 登録名とメモを反映し、見えていない登録済み機器を加える
	byMAC := map[string]store.Alias{}
	for _, a := range aliases {
		if a.MatchType == "mac" {
			byMAC[strings.ToUpper(a.MatchKey)] = a
		}
	}
	for i := range rows {
		if a, ok := byMAC[rows[i].MAC]; ok {
			rows[i].Label, rows[i].Note = a.Label, a.Note
		}
	}
	for mac, a := range byMAC {
		if _, ok := best[mac]; ok {
			continue
		}
		rows = append(rows, InventoryRow{MAC: mac, Label: a.Label, Note: a.Note, LastSeen: lastSeen[mac]})
	}
	out := rows[:0]
	q := strings.ToLower(strings.TrimSpace(opt.Query))
	for _, r := range rows {
		if opt.Scope == "unnamed" && r.Label != "" {
			continue
		}
		// 担当パートで絞るとき、所在不明（未検出）の登録機器はどのパートか判断できないので除く
		if opt.Group != "" && r.GroupTag != opt.Group {
			continue
		}
		if len(opt.VLANs) > 0 && !containsInt(opt.VLANs, r.VLAN) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(strings.Join([]string{r.MAC, r.IP, r.Label, r.Note, r.Vendor, r.LLDPName, r.SwitchName, r.Port, strconv.Itoa(r.VLAN)}, "\x00")), q) {
			continue
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Seen != b.Seen {
			return a.Seen
		}
		if a.SwitchName != b.SwitchName {
			return a.SwitchName < b.SwitchName
		}
		if a.IfIndex != b.IfIndex {
			return a.IfIndex < b.IfIndex
		}
		return a.MAC < b.MAC
	})
	return out
}

// InventoryCSVHeader は書出・読込共通の列。読込は先頭3列（MAC・名称・メモ）だけを使う。
var InventoryCSVHeader = []string{"MAC", "名称", "メモ", "IP", "メーカー", "LLDP名", "スイッチ", "ポート", "VLAN", "PoE(W)", "リンク速度", "最終確認", "検出元"}

// WriteInventoryCSV はExcelでそのまま開けるよう BOM付きUTF-8・CRLF で書く。loc は時刻表示のタイムゾーン。
func WriteInventoryCSV(w io.Writer, rows []InventoryRow, loc *time.Location) error {
	if _, err := io.WriteString(w, "\uFEFF"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.UseCRLF = true
	if err := cw.Write(InventoryCSVHeader); err != nil {
		return err
	}
	for _, r := range rows {
		poe, speed, vlan, last, src := "", "", "", "未検出", r.Source
		if r.PoEW != nil {
			poe = strconv.FormatFloat(*r.PoEW, 'f', 1, 64)
		}
		if r.SpeedMbps > 0 {
			speed = formatSpeed(r.SpeedMbps)
		}
		if r.VLAN > 0 {
			vlan = strconv.Itoa(r.VLAN)
		}
		if !r.LastSeen.IsZero() {
			last = r.LastSeen.In(loc).Format("2006-01-02 15:04:05")
			if !r.Seen {
				last += "（未検出）"
			}
		}
		if r.ViaUplink {
			src += "(経路上)"
		}
		rec := []string{r.MAC, r.Label, r.Note, r.IP, r.Vendor, r.LLDPName, r.SwitchName, r.Port, vlan, poe, speed, last, src}
		for i := range rec {
			rec[i] = csvSafe(rec[i])
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func formatSpeed(mbps int) string {
	if mbps >= 1000 && mbps%1000 == 0 {
		return strconv.Itoa(mbps/1000) + "G"
	}
	return strconv.Itoa(mbps) + "M"
}

// csvSafe は表計算ソフトで数式として解釈される先頭文字を無害化する（CSVインジェクション対策）。
// LLDP名などはネットワーク上の機器が自由に名乗る未信頼値のため、全セルに適用する。
func csvSafe(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune("	", rune(s[0])) {
		return "'" + s
	}
	if strings.ContainsRune("=+-@", rune(s[0])) {
		if _, err := strconv.ParseFloat(s, 64); err != nil { // 負の数値はそのまま（数値として開けるように）
			return "'" + s
		}
	}
	return s
}

// ---- 機器名CSVの読込 ----

// MaxAliasCSVRows は1回の読込で受け付けるデータ行数の上限。
const MaxAliasCSVRows = 5000

// AliasCSVRow は読込CSVの1データ行。
type AliasCSVRow struct {
	Line  int    `json:"line"`
	MAC   string `json:"mac"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// AliasCSVError は解釈できなかった行。
type AliasCSVError struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// ParseAliasCSV は機器名CSVを解釈する。名称が空の行は skipped に数えて無視する（雛形の未記入行）。
// 見出し行があれば列名で MAC / 名称 / メモ の位置を決め、なければ先頭3列とみなす。
func ParseAliasCSV(text string) (rows []AliasCSVRow, skipped int, errs []AliasCSVError) {
	text = strings.TrimPrefix(text, "\uFEFF")
	cr := csv.NewReader(strings.NewReader(text))
	cr.FieldsPerRecord = -1 // 列数の揃わない行（末尾の空列省略など）も受け付ける
	cr.LazyQuotes = true
	colMAC, colLabel, colNote := 0, 1, 2
	seen := map[string]int{}
	first := true
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		line, _ := cr.FieldPos(0)
		if err != nil {
			errs = append(errs, AliasCSVError{Line: line, Reason: "CSVとして読めません: " + err.Error()})
			break
		}
		if first {
			first = false
			if _, ok := NormalizeMAC(cell(rec, 0)); !ok {
				if m, l, n, ok := headerColumns(rec); ok {
					colMAC, colLabel, colNote = m, l, n
					continue
				}
			}
		}
		if isBlank(rec) {
			continue
		}
		if len(rows)+skipped+len(errs) >= MaxAliasCSVRows {
			errs = append(errs, AliasCSVError{Line: line, Reason: fmt.Sprintf("行数が上限（%d行）を超えています。ファイルを分けて読み込んでください", MaxAliasCSVRows)})
			break
		}
		mac, ok := NormalizeMAC(cell(rec, colMAC))
		if !ok {
			errs = append(errs, AliasCSVError{Line: line, Reason: fmt.Sprintf("MACアドレスとして読めません: %q", truncateRunes(cell(rec, colMAC), 40))})
			continue
		}
		label := unCSVSafe(strings.TrimSpace(cell(rec, colLabel)))
		note := unCSVSafe(strings.TrimSpace(cell(rec, colNote)))
		if label == "" {
			skipped++
			continue
		}
		if len([]rune(label)) > 64 || len([]rune(note)) > 200 {
			errs = append(errs, AliasCSVError{Line: line, Reason: "名称は64文字、メモは200文字までです"})
			continue
		}
		if prev, dup := seen[mac]; dup {
			errs = append(errs, AliasCSVError{Line: line, Reason: fmt.Sprintf("MACアドレス %s が %d行目と重複しています", mac, prev)})
			continue
		}
		seen[mac] = line
		rows = append(rows, AliasCSVRow{Line: line, MAC: mac, Label: label, Note: note})
	}
	return rows, skipped, errs
}

// headerColumns は見出し行から列位置を探す。MAC列と名称列が両方見つかったときだけ見出しとみなす。
func headerColumns(rec []string) (mac, label, note int, ok bool) {
	mac, label, note = -1, -1, -1
	for i, h := range rec {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF")))
		switch {
		case mac < 0 && (h == "mac" || strings.Contains(h, "macアドレス") || h == "mac address"):
			mac = i
		case label < 0 && (h == "名称" || h == "機器名" || h == "名前" || h == "name" || h == "label"):
			label = i
		case note < 0 && (h == "メモ" || h == "備考" || h == "note" || h == "memo"):
			note = i
		}
	}
	return mac, label, note, mac >= 0 && label >= 0
}

// NormalizeMAC は区切りの異なる表記を AA:BB:CC:DD:EE:FF にそろえる。
func NormalizeMAC(s string) (string, bool) {
	var hex []byte
	for _, c := range []byte(strings.TrimSpace(s)) {
		switch {
		case c == ':' || c == '-' || c == '.' || c == ' ':
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
			hex = append(hex, c)
		default:
			return "", false
		}
	}
	if len(hex) != 12 {
		return "", false
	}
	u := strings.ToUpper(string(hex))
	return u[0:2] + ":" + u[2:4] + ":" + u[4:6] + ":" + u[6:8] + ":" + u[8:10] + ":" + u[10:12], true
}

func cell(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func isBlank(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

// unCSVSafe は書出時に付けた無害化の ' を外す（書出→編集→読込の往復で名称が変わらないように）。
func unCSVSafe(s string) string {
	if len(s) >= 2 && s[0] == '\'' && strings.ContainsRune("=+-@", rune(s[1])) {
		return s[1:]
	}
	return s
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// Inventory はライブ状態の接続機器一覧。
func (e *Engine) Inventory(opt InventoryOptions) ([]InventoryRow, error) {
	al, err := e.st.Aliases(e.ProfileID())
	if err != nil {
		return nil, err
	}
	last, err := e.st.LastSeenByMAC()
	if err != nil {
		return nil, err
	}
	return BuildInventory(e.States(), al, last, opt, time.Now()), nil
}
