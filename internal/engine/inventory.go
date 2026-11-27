package engine

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
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
	PlannedIP  string     `json:"planned_ip"`
	IPMismatch bool       `json:"ip_mismatch"`
	PingOK     *bool      `json:"ping_ok"`
	PingRTTms  *float64   `json:"ping_rtt_ms"`
	PingTS     *time.Time `json:"ping_ts"`
	MAC        string     `json:"mac"`
	Label      string     `json:"label"`
	Note       string     `json:"note"`
	IP         string     `json:"ip"`
	Vendor     string     `json:"vendor"`
	LLDPName   string     `json:"lldp_name"`
	SwitchID   int64      `json:"switch_id,omitempty"`
	SwitchName string     `json:"switch_name"`
	GroupTag   string     `json:"group_tag"`
	Port       string     `json:"port"`
	IfIndex    int        `json:"if_index,omitempty"`
	VLAN       int        `json:"vlan"`
	PoEW       *float64   `json:"poe_w,omitempty"`
	SpeedMbps  int        `json:"speed_mbps"`
	Seen       bool       `json:"seen"`                // 現在（または再生時刻に）見えているか
	LastSeen   time.Time  `json:"last_seen,omitempty"` // 見えていない登録済み機器の最終観測（不明ならゼロ）
	IPStale    bool       `json:"ip_stale,omitempty"`  // IP は最終観測時のもの（現在も同じとは限らない）
	Source     string     `json:"source"`
	ViaUplink  bool       `json:"via_uplink"`           // スイッチ間リンクでしか見えていない（所在は経路上の推定）
	PortTrunk  bool       `json:"port_trunk,omitempty"` // つながっているポートが Trunk
	PortLag    string     `json:"port_lag,omitempty"`   // つながっているポートが属する LAG の名前
	Ping       *PingMark  `json:"ping,omitempty"`       // このIPへの直近の一括Ping結果（ライブのみ）
}

// PingMark はIPごとの直近Ping結果（一覧とCSVに載せるため engine が保持する。再起動で消える）。
type PingMark struct {
	Status string    `json:"status"` // ok / timeout / unreachable / error
	RTTms  float64   `json:"rtt_ms"`
	At     time.Time `json:"at"`
}

// InventoryOptions は一覧の絞込条件。
type InventoryOptions struct {
	Scope string // "all"（既定）/ "unnamed"
	Group string // 担当パート（スイッチの group_tag）。空なら全体
	Query string // MAC / IP / 名称 / メーカー / LLDP名 / スイッチ / ポート の部分一致
	VLANs []int  // 担当パート（表示プリセット）のVLAN。空なら全VLAN
	// SnapshotLabels: 再生用。スナップショットに記録された名称を登録名より優先する
	SnapshotLabels bool
}

// BuildInventory は状態から MAC ごとに1行の一覧を作る。
// at は「見えている」機器の最終確認時刻（ライブは現在、再生はスナップショット時刻）。
// lastSeen は見えていない登録済み機器の最終観測（device_bindings 由来、無ければ欠落）。
func BuildInventory(states []*model.SwitchState, aliases []store.Alias, lastSeen map[string]store.LastSeen, opt InventoryOptions, at time.Time) []InventoryRow {
	type cand struct {
		row    InventoryRow
		uplink bool
		lldp   bool
		live   bool // スイッチから現在取得できている観測か
	}
	best := map[string]cand{}
	// 所在とは別に、MAC単位で判明している属性を集める（所在に選ばれた観測でIPが不明でも、他の観測のIPを使う）
	// IPは「稼働中スイッチの観測」→「アクセスポートの観測」の順に信頼する（到達不能スイッチに残る古いIPより新しいIPを使う）
	type ipCand struct {
		ip   string
		rank int
	}
	anyIP := map[string]ipCand{}
	anyVendor, anyLLDP := map[string]string{}, map[string]string{}
	better := func(a, b cand) bool { // a が b より所在として確からしいか
		if a.live != b.live {
			return a.live
		}
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
		// 到達不能なスイッチの機器一覧はポーラーが最後に取れた時点のもの。見えている扱いにしない
		live := s.Reachable
		seenAt := at
		if !live {
			seenAt = s.LastOK
		}
		for _, p := range s.Ports {
			up := isUplink(s, p)
			for _, d := range p.Devices {
				if d.IP != "" {
					rank := 0
					if live {
						rank += 2
					}
					if !up {
						rank++
					}
					if cur, ok := anyIP[d.MAC]; !ok || rank > cur.rank {
						anyIP[d.MAC] = ipCand{d.IP, rank}
					}
				}
				if d.Vendor != "" {
					anyVendor[d.MAC] = d.Vendor
				}
				if d.LLDPName != "" {
					anyLLDP[d.MAC] = d.LLDPName
				}
				r := InventoryRow{MAC: d.MAC, Label: d.Label, IP: d.IP, Vendor: d.Vendor, LLDPName: d.LLDPName,
					SwitchID: s.ID, SwitchName: s.Name, GroupTag: s.GroupTag, Port: p.Name, IfIndex: p.IfIndex,
					VLAN: d.VLAN, SpeedMbps: p.SpeedMbps, Seen: live, LastSeen: seenAt, Source: d.Source, ViaUplink: up,
					PortTrunk: p.Mode == "trunk", PortLag: lagLabel(p)}
				if r.VLAN == 0 {
					r.VLAN = p.PVID
				}
				// 1ポートに複数機器（配下にハブ）なら、ポートのPoE値は機器ごとの値ではないため出さない
				if p.PoE != nil && len(p.Devices) == 1 {
					w := p.PoE.PowerW
					r.PoEW = &w
				}
				c := cand{row: r, uplink: up, lldp: d.Source == "lldp", live: live}
				if old, ok := best[d.MAC]; !ok || better(c, old) {
					best[d.MAC] = c
				}
			}
		}
	}
	rows := make([]InventoryRow, 0, len(best)+len(aliases))
	for mac, c := range best {
		r := c.row
		if r.IP == "" && anyIP[mac].ip == "" && lastSeen[mac].IP != "" {
			r.IP = lastSeen[mac].IP
			r.IPStale = true
		}
		if r.IP == "" || !c.live {
			if ip := anyIP[mac].ip; ip != "" {
				r.IP = ip
			}
		}
		if r.Vendor == "" {
			r.Vendor = anyVendor[mac]
		}
		if r.LLDPName == "" {
			r.LLDPName = anyLLDP[mac]
		}
		rows = append(rows, r)
	}
	// 登録名とメモを反映し、見えていない登録済み機器を加える
	byMAC := map[string]store.Alias{}
	for _, a := range aliases {
		if a.MatchType == "mac" {
			byMAC[strings.ToUpper(a.MatchKey)] = a
		}
	}
	portLabel := map[string]string{}
	for _, a := range aliases {
		if a.MatchType == "port" {
			portLabel[a.MatchKey] = a.Label
		}
	}
	for i := range rows {
		a, ok := byMAC[rows[i].MAC]
		if ok {
			rows[i].Note = a.Note
			rows[i].PlannedIP = a.PlannedIP
			rows[i].IPMismatch = a.PlannedIP != "" && rows[i].IP != "" && a.PlannedIP != rows[i].IP
		}
		// 再生ではスナップショット記録時点の名称を、空も含めてそのまま保つ（後日の登録名で過去を書き換えない）
		if opt.SnapshotLabels {
			continue
		}
		// ライブは現在の登録から毎回引き直す（状態に残った古い名称は、削除後も残り続けるため使わない）
		switch {
		case ok:
			rows[i].Label = a.Label
		case rows[i].SwitchID != 0:
			rows[i].Label = portLabel[portKey(rows[i].SwitchID, rows[i].IfIndex)]
		default:
			rows[i].Label = ""
		}
	}
	for mac, a := range byMAC {
		if _, ok := best[mac]; ok {
			continue
		}
		ls := lastSeen[mac]
		r := InventoryRow{MAC: mac, Label: a.Label, Note: a.Note, LastSeen: ls.TS, IP: ls.IP, IPStale: ls.IP != "", PlannedIP: a.PlannedIP,
			IPMismatch: a.PlannedIP != "" && ls.IP != "" && a.PlannedIP != ls.IP, SwitchID: ls.SwitchID, IfIndex: ls.IfIndex}
		for _, s := range states {
			if s.ID == ls.SwitchID {
				r.SwitchName, r.GroupTag = s.Name, s.GroupTag
				for _, p := range s.Ports {
					if p.IfIndex == ls.IfIndex {
						r.Port = p.Name
						r.PortTrunk, r.PortLag = p.Mode == "trunk", lagLabel(p)
					}
				}
				break
			}
		}
		rows = append(rows, r)
	}
	out := rows[:0]
	q := strings.ToLower(strings.TrimSpace(opt.Query))
	for _, r := range rows {
		if opt.Scope == "unnamed" && r.Label != "" {
			continue
		}
		// 担当パートで絞るとき、所在不明（未検出）の登録機器はどのパートか判断できないので除く
		if opt.Group != "" && opt.Group != "*" && r.GroupTag != opt.Group {
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

// InventoryCSVHeader は書出・読込共通の列。先頭4列は機器名CSVとして再読込できる。
var InventoryCSVHeader = []string{"MAC", "名称", "メモ", "予定IP", "IP", "メーカー", "LLDP名", "スイッチ", "ポート", "VLAN", "PoE(W)", "リンク速度", "最終確認", "検出元", "Ping"}

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
		rec := []string{r.MAC, r.Label, r.Note, r.PlannedIP, r.IP, r.Vendor, r.LLDPName, r.SwitchName, r.Port, vlan, poe, speed, last, src, pingCell(r.Ping, loc)}
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

// pingCell は「応答 3ms (14:02:05)」「無応答 (14:02:05)」の形にする。未実施は空欄。
func pingCell(p *PingMark, loc *time.Location) string {
	if p == nil {
		return ""
	}
	label := map[string]string{"timeout": "無応答", "unreachable": "到達不能", "error": "送信失敗"}[p.Status]
	if p.Status == "ok" {
		label = "応答 " + strconv.FormatFloat(p.RTTms, 'f', 0, 64) + "ms"
	}
	return label + " (" + p.At.In(loc).Format("15:04:05") + ")"
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
	// 元から ' で始まる値にも ' を足す（読込時に1つだけ外すので、書出→読込で名称が変わらない）
	if s[0] == '\t' || s[0] == '\r' || s[0] == '\'' {
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
	PlannedIP string `json:"planned_ip"`
	Line      int    `json:"line"`
	MAC       string `json:"mac"`
	Label     string `json:"label"`
	Note      string `json:"note"`
}

// AliasCSVError は解釈できなかった行。
type AliasCSVError struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// ParseAliasCSV は機器名CSVを解釈する。名称が空の行は skipped に数えて無視する（雛形の未記入行）。
// 見出し行があれば列名で判定し、なければ先頭4列とみなす（予定IPの欠落は空欄）。
func ParseAliasCSV(text string) (rows []AliasCSVRow, skipped int, errs []AliasCSVError) {
	text = strings.TrimPrefix(text, "\uFEFF")
	cr := csv.NewReader(strings.NewReader(text))
	cr.FieldsPerRecord = -1 // 列数の揃わない行（末尾の空列省略など）も受け付ける
	// 引用符は厳密に扱う。閉じ忘れを許すと後続行がメモに吸収され、置換モードで登録名を消してしまう
	colMAC, colLabel, colNote, colPlanned := 0, 1, 2, 3
	seen := map[string]int{}
	first := true
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// 構文エラーの行では FieldPos が使えない（フィールドが無いと panic する）ため ParseError から行を取る
			line := 0
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				line = pe.StartLine
			}
			errs = append(errs, AliasCSVError{Line: line, Reason: "CSVとして読めません: " + err.Error()})
			break
		}
		line, _ := cr.FieldPos(0)
		if first {
			first = false
			if _, ok := NormalizeMAC(cell(rec, 0)); !ok {
				if m, l, n, ok := headerColumns(rec); ok {
					colMAC, colLabel, colNote = m, l, n
					colPlanned = -1
					for i, h := range rec {
						if strings.TrimSpace(h) == "予定IP" || strings.EqualFold(strings.TrimSpace(h), "planned_ip") {
							colPlanned = i
							break
						}
					}
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
		planned := strings.TrimSpace(cell(rec, colPlanned))
		if !ValidPlannedIP(planned) {
			errs = append(errs, AliasCSVError{Line: line, Reason: "予定IPはIPv4アドレスか空欄にしてください"})
			continue
		}
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
		rows = append(rows, AliasCSVRow{Line: line, MAC: mac, Label: label, Note: note, PlannedIP: planned})
	}
	return rows, skipped, errs
}

func ValidPlannedIP(ip string) bool {
	if ip == "" {
		return true
	}
	addr, err := netip.ParseAddr(ip)
	return err == nil && addr.Is4()
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
	if len(s) >= 2 && s[0] == '\'' && strings.ContainsRune("=+-@'\t", rune(s[1])) {
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
	last, err := e.st.LastSeenByMAC(e.ProfileID(), time.Now())
	if err != nil {
		return nil, err
	}
	rows := BuildInventory(e.States(), al, last, opt, time.Now())
	e.mu.RLock()
	for i := range rows {
		if p, ok := e.pingResults[rows[i].MAC]; ok {
			p := p
			rows[i].PingOK, rows[i].PingRTTms, rows[i].PingTS = &p.OK, &p.RTTms, &p.TS
			rows[i].Ping = &PingMark{Status: p.Status, RTTms: p.RTTms, At: p.TS}
		}
	}
	e.mu.RUnlock()
	return rows, nil
}

// RecordPing は一括Pingの結果を保持する（IPごとに最新のみ）。
func (e *Engine) RecordPing(results map[string]PingMark) {
	rows, err := e.Inventory(InventoryOptions{})
	if err != nil {
		slog.Warn("Ping結果の機器照合に失敗", "err", err)
		return
	}
	e.mu.Lock()
	recovered := map[string]bool{}
	known := map[string]bool{}
	for _, r := range rows {
		known[r.MAC] = true
	}
	for mac := range e.pingResults {
		if !known[mac] {
			delete(e.pingResults, mac)
		}
	}
	for ip, m := range results {
		for _, r := range rows {
			if r.IP == ip || r.PlannedIP == ip {
				e.pingResults[r.MAC] = DevicePing{IP: ip, OK: m.Status == "ok", RTTms: m.RTTms, TS: m.At, Source: "manual", Status: m.Status}
				if m.Status == "ok" {
					e.pingFails[r.MAC] = 0
					recovered[r.MAC] = true
				}
			}
		}
	}
	e.mu.Unlock()
	e.clearMatching(func(a *Alert) bool { return a.Code == "PING_FAIL" && recovered[a.MAC] })
}

// lagLabel は LAG に属するポートなら LAG 名（名前が取れなければ ifIndex）を返す。
func lagLabel(p *model.Port) string {
	if p.LagIfIndex <= 0 {
		return ""
	}
	if p.LagName != "" {
		return p.LagName
	}
	return "ifIndex " + itoa(p.LagIfIndex)
}
