package api

import (
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/ping"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func netInterfaces() ([]map[string]any, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := i.Addrs()
		var as []string
		for _, a := range addrs {
			as = append(as, a.String())
		}
		out = append(out, map[string]any{"name": i.Name, "addrs": as})
	}
	return out, nil
}

// ---- スイッチ ----

func (s *Server) listSwitches(w http.ResponseWriter, r *http.Request) {
	sws, err := s.Store.Switches(s.Engine.ProfileID())
	if err != nil {
		writeErr(w, 500, "スイッチ一覧を取得できません", err.Error(), "")
		return
	}
	out := []model.SwitchConfig{}
	for _, c := range sws {
		c.Community = "****" // コミュニティ文字列はAPIで返さない
		out = append(out, c)
	}
	writeJSON(w, 200, out)
}

func validHost(ip string) bool {
	if net.ParseIP(ip) != nil {
		return true
	}
	return regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,252}$`).MatchString(ip)
}

func (s *Server) saveSwitch(w http.ResponseWriter, r *http.Request) {
	var in model.SwitchConfig
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "スイッチ設定が不正です", err.Error(), "")
		return
	}
	in.IP = strings.TrimSpace(in.IP)
	if strings.TrimSpace(in.Name) == "" || !validHost(in.IP) {
		writeErr(w, 400, "スイッチを保存できません", "名前またはIPアドレスが不正です", "名前とIPv4アドレス（例 192.168.0.239）を入力してください")
		return
	}
	if in.SNMPPort < 0 || in.SNMPPort > 65535 {
		writeErr(w, 400, "スイッチを保存できません", "SNMPポート番号が範囲外です", "通常は161です")
		return
	}
	pid := s.Engine.ProfileID()
	if r.Method == http.MethodPut {
		id, err := pathID(r, "id")
		if err != nil {
			writeErr(w, 400, "不正なID", "", "")
			return
		}
		in.ID = id
		// "****" または空ならコミュニティは変更しない
		if in.Community == "" || in.Community == "****" {
			old, _ := s.Store.Switches(pid)
			for _, o := range old {
				if o.ID == id {
					in.Community = o.Community
				}
			}
		}
	} else {
		in.ID = 0
		if in.Community == "" {
			in.Community = "public"
		}
		if !in.Enabled {
			in.Enabled = true
		}
	}
	in.ProfileID = pid
	if err := s.Store.SaveSwitch(&in); err != nil {
		writeErr(w, 500, "スイッチを保存できません", err.Error(), "")
		return
	}
	s.resync()
	in.Community = "****"
	writeJSON(w, 200, in)
}

func (s *Server) deleteSwitch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err == nil {
		err = s.Store.DeleteSwitch(id)
	}
	if err != nil {
		writeErr(w, 400, "スイッチを削除できません", err.Error(), "")
		return
	}
	s.resync()
	writeJSON(w, 200, map[string]any{"deleted": id})
}

func (s *Server) switchState(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	st := s.Engine.State(id)
	if st == nil {
		writeErr(w, 404, "スイッチが見つかりません", "", "一覧を再読込してください")
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) portSetting(w http.ResponseWriter, r *http.Request) {
	id, err1 := pathID(r, "id")
	ix, err2 := strconv.Atoi(r.PathValue("ifIndex"))
	if err1 != nil || err2 != nil {
		writeErr(w, 400, "不正なパラメータ", "", "")
		return
	}
	cur, _ := s.Store.PortSettings(id)
	ps := cur[ix]
	var in map[string]json.RawMessage
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "ポート設定が不正です", err.Error(), "")
		return
	}
	if v, ok := in["memo"]; ok {
		json.Unmarshal(v, &ps.Memo)
	}
	if v, ok := in["excluded"]; ok {
		json.Unmarshal(v, &ps.Excluded)
	}
	if v, ok := in["expected_speed_mbps"]; ok {
		json.Unmarshal(v, &ps.ExpectedSpeed)
	}
	if v, ok := in["av_role"]; ok {
		json.Unmarshal(v, &ps.AVRole)
	}
	if err := s.Store.SavePortSetting(id, ix, ps); err != nil {
		writeErr(w, 500, "ポート設定を保存できません", err.Error(), "")
		return
	}
	s.Engine.Reload()
	writeJSON(w, 200, ps)
}

func (s *Server) resetCounters(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	var in struct {
		IfIndex int `json:"if_index"`
	}
	readJSON(r, &in)
	s.Poller.ResetCounters(id, in.IfIndex)
	writeJSON(w, 200, map[string]any{"reset_at": time.Now().UTC(), "if_index": in.IfIndex})
}

func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	s.Poller.Probe(id)
	writeJSON(w, 200, map[string]string{"status": "プローブを開始しました。次回のSlow周期で結果が反映されます"})
}

func (s *Server) findSwitch(id int64) (model.SwitchConfig, bool) {
	sws, _ := s.Store.Switches(s.Engine.ProfileID())
	for _, c := range sws {
		if c.ID == id {
			return c, true
		}
	}
	return model.SwitchConfig{}, false
}

// diagDump は実機が返したOID構造・型・生値をJSONで出力する（仕様7.2）。community はマスク。
func (s *Server) diagDump(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	cfg, ok := s.findSwitch(id)
	if !ok {
		writeErr(w, 404, "スイッチが見つかりません", "", "")
		return
	}
	c, err := sx.Dial(cfg.IP, uint16(cfg.SNMPPort), cfg.Community, 2*time.Second, 1)
	if err != nil {
		writeErr(w, 502, "診断ダンプを取得できません", err.Error(), "")
		return
	}
	defer c.Close()
	groups := map[string][]string{
		"system": {mib.SysDescr, mib.SysObjectID, mib.SysUpTime, mib.SysName, mib.AgentInventoryMachineModel, mib.AgentInventorySoftwareVersion},
	}
	tables := map[string][]string{
		"supported_mibs": {mib.AgentSupportedMibName}, "if": {mib.IfName, mib.IfType, mib.IfOperStatus, mib.IfHighSpeed},
		"port_rate": {mib.PortRateHCBpsRx}, "etherlike": {mib.Dot3FCSErrors, mib.Dot3PauseOperMode}, "poe": {mib.PethPortDetectionStatus, mib.AgentPethOutputPower},
		"box_temp": {mib.TempSensorTemp, mib.TempUnitState}, "box_fan": {mib.FanItemState, mib.FanSpeed}, "box_psu": {mib.PowSupplyItemState},
		"sfp_dom": {mib.OpticsPowerIn, mib.OpticsPowerOut}, "igmp_querier": {mib.QuerierVlanOperMode, mib.QuerierLastQuerierAddress},
		"igmp_ssm": {mib.SSMGroupLastReporter}, "lldp": {mib.LldpRemSysName, mib.LldpRemChassisID}, "fdb": {mib.Dot1qTpFdbPort},
		"dhcp_snoop_binding": {mib.DsBindingIfIndex}, "green": {mib.GreenEeeMode}, "udld": {mib.UdldIntfOperStatus},
		"cos": {mib.CosQueueIntfHCTotalDrop}, "mbuf": {mib.MbufAllocFails}, "log": {mib.LogInMemoryMsgText},
	}
	dump := map[string]any{"switch": cfg.Name, "ip": cfg.IP, "community": "****", "generated": time.Now().UTC()}
	pduJSON := func(p gosnmp.SnmpPDU) map[string]any {
		v := any(p.Value)
		if b, ok := p.Value.([]byte); ok {
			v = map[string]string{"hex": hex.EncodeToString(b), "text": sx.Str(p)}
		}
		return map[string]any{"oid": strings.TrimPrefix(p.Name, "."), "type": p.Type.String(), "value": v}
	}
	for g, oids := range groups {
		res, err := c.Get(oids)
		if err != nil {
			dump[g] = map[string]string{"error": err.Error()}
			continue
		}
		var out []map[string]any
		for _, o := range oids {
			if p, ok := res[o]; ok {
				out = append(out, pduJSON(p))
			} else {
				out = append(out, map[string]any{"oid": o, "status": "UNSUPPORTED"})
			}
		}
		dump[g] = out
	}
	for g, cols := range tables {
		rows, err := c.WalkColumns(cols)
		if err != nil {
			dump[g] = map[string]string{"status": "ERROR", "error": err.Error()}
			continue
		}
		var out []map[string]any
		for i, row := range rows {
			if i >= 60 {
				break // ダンプ肥大化防止
			}
			for _, col := range cols {
				if p, ok := row.Cols[col]; ok {
					out = append(out, pduJSON(p))
				}
			}
		}
		status := "SUPPORTED"
		if len(rows) == 0 {
			status = "UNSUPPORTED"
		}
		dump[g] = map[string]any{"status": status, "rows": len(rows), "sample": out}
	}
	if st := s.Engine.State(id); st != nil {
		dump["capabilities"] = st.Caps
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="diag-%s.json"`, sanitizeName(cfg.Name)))
	writeJSON(w, 200, dump)
}

func sanitizeName(s string) string {
	return regexp.MustCompile(`[^A-Za-z0-9_.-]`).ReplaceAllString(s, "_")
}

func (s *Server) accessInfo(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	cfg, ok := s.findSwitch(id)
	if !ok {
		writeErr(w, 404, "スイッチが見つかりません", "", "")
		return
	}
	st := s.Engine.State(id)
	url := cfg.WebURL
	https, sshOn, sshPort, telPort := true, false, 22, 23
	if st != nil && st.Reachable {
		https, sshOn = st.Config.HTTPS, st.Config.SSH
		if st.Config.SSHPort > 0 {
			sshPort = st.Config.SSHPort
		}
		if st.Config.TelnetPort > 0 {
			telPort = st.Config.TelnetPort
		}
	}
	if url == "" {
		if https {
			port := 443
			if st != nil && st.Config.HTTPSPort > 0 {
				port = st.Config.HTTPSPort
			}
			url = "https://" + cfg.IP
			if port != 443 {
				url += ":" + strconv.Itoa(port)
			}
		} else {
			url = "http://" + cfg.IP
		}
	}
	writeJSON(w, 200, map[string]any{"web_url": url, "https": https, "ssh": map[string]any{"enabled": sshOn, "port": sshPort}, "telnet": map[string]any{"port": telPort}})
}

func (s *Server) saveAlias(w http.ResponseWriter, r *http.Request) {
	var a store.Alias
	if err := readJSON(r, &a); err != nil || (a.MatchType != "mac" && a.MatchType != "port") || a.MatchKey == "" {
		writeErr(w, 400, "ラベルを保存できません", "match_type は mac か port、match_key が必要です", "")
		return
	}
	a.MatchKey = strings.ToUpper(a.MatchKey)
	if err := s.Store.SaveAlias(s.Engine.ProfileID(), a); err != nil {
		writeErr(w, 500, "ラベルを保存できません", err.Error(), "")
		return
	}
	s.Engine.Reload()
	writeJSON(w, 200, a)
}

// ---- 監視ビュー ----

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.Engine.Overview(true))
}

func (s *Server) searchPorts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sw, _ := strconv.ParseInt(q.Get("switch_id"), 10, 64)
	writeJSON(w, 200, s.Engine.SearchPorts(q.Get("q"), sw, q.Get("sort")))
}

func timeRange(r *http.Request, def time.Duration) (time.Time, time.Time) {
	to := time.Now().UTC()
	from := to.Add(-def)
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}

func (s *Server) portMetrics(w http.ResponseWriter, r *http.Request) {
	sw, _ := strconv.ParseInt(r.URL.Query().Get("switch_id"), 10, 64)
	ix, _ := strconv.Atoi(r.URL.Query().Get("if_index"))
	from, to := timeRange(r, 30*time.Minute)
	res, pts, err := s.Store.PortSeries(sw, ix, from, to)
	if err != nil {
		writeErr(w, 500, "時系列を取得できません", err.Error(), "")
		return
	}
	if pts == nil {
		pts = []store.Point{}
	}
	writeJSON(w, 200, map[string]any{"resolution": res, "points": pts})
}

func (s *Server) switchMetrics(w http.ResponseWriter, r *http.Request) {
	sw, _ := strconv.ParseInt(r.URL.Query().Get("switch_id"), 10, 64)
	from, to := timeRange(r, time.Hour)
	res, pts, err := s.Store.SwitchSeries(sw, from, to)
	if err != nil {
		writeErr(w, 500, "時系列を取得できません", err.Error(), "")
		return
	}
	if pts == nil {
		pts = []store.Point{}
	}
	writeJSON(w, 200, map[string]any{"resolution": res, "points": pts})
}

type xy = struct{ X, Y float64 }

func (s *Server) topology(w http.ResponseWriter, r *http.Request) {
	lay := map[string]xy{}
	s.Store.ProfileJSON(s.Engine.ProfileID(), "topology_layout_json", &lay)
	t := s.Engine.Topology(lay)
	if b, _, err := s.Store.Background(s.Engine.ProfileID()); err == nil && len(b) > 0 {
		u := "/api/topology/background?v=" + strconv.Itoa(len(b))
		t.Background = &u
	}
	writeJSON(w, 200, t)
}

func (s *Server) topologyLayout(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Positions map[string]xy `json:"positions"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "レイアウトを保存できません", err.Error(), "")
		return
	}
	if err := s.Store.SetProfileJSON(s.Engine.ProfileID(), "topology_layout_json", in.Positions); err != nil {
		writeErr(w, 500, "レイアウトを保存できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]int{"saved": len(in.Positions)})
}

func (s *Server) uploadBackground(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "背景画像を読み込めません", err.Error(), "10MB以下のPNG/JPEG/SVGを指定してください")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		writeErr(w, 400, "背景画像を読み込めません", err.Error(), "")
		return
	}
	ct := http.DetectContentType(data)
	if strings.HasSuffix(strings.ToLower(hdr.Filename), ".svg") {
		ct = "image/svg+xml"
	}
	if !strings.HasPrefix(ct, "image/") {
		writeErr(w, 400, "背景画像を読み込めません", "画像ファイルではありません（"+ct+"）", "PNG/JPEG/SVGを指定してください")
		return
	}
	if err := s.Store.SetBackground(s.Engine.ProfileID(), data, ct); err != nil {
		writeErr(w, 500, "背景画像を保存できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]string{"url": "/api/topology/background?v=" + strconv.Itoa(len(data))})
}

func (s *Server) getBackground(w http.ResponseWriter, r *http.Request) {
	b, ct, err := s.Store.Background(s.Engine.ProfileID())
	if err != nil || len(b) == 0 {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	// SVG内スクリプトを無効化
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	w.Write(b)
}

func (s *Server) getWatchlist(w http.ResponseWriter, r *http.Request) {
	var wl []map[string]any
	s.Store.ProfileJSON(s.Engine.ProfileID(), "watchlist_json", &wl)
	if wl == nil {
		wl = []map[string]any{}
	}
	writeJSON(w, 200, wl)
}

func (s *Server) putWatchlist(w http.ResponseWriter, r *http.Request) {
	var wl []map[string]any
	if err := readJSON(r, &wl); err != nil {
		writeErr(w, 400, "Watch Listを保存できません", err.Error(), "")
		return
	}
	s.Store.SetProfileJSON(s.Engine.ProfileID(), "watchlist_json", wl)
	writeJSON(w, 200, wl)
}

func (s *Server) audit(w http.ResponseWriter, r *http.Request) {
	fs, fw := s.Engine.Audit()
	writeJSON(w, 200, map[string]any{"findings": fs, "fw_versions": fw})
}

func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IP string `json:"ip"`
	}
	if err := readJSON(r, &in); err != nil || net.ParseIP(in.IP).To4() == nil {
		writeErr(w, 400, "Pingできません", "IPv4アドレスが不正です", "")
		return
	}
	if !s.pingAllowed()[in.IP] {
		writeErr(w, 403, "Pingできません", "登録スイッチ・観測済み機器・登録機器の最終IPではありません", "任意のIPは接続機器画面の一括Pingで手入力してください")
		return
	}
	start := time.Now()
	res := ping.One(r.Context(), in.IP, ping.DefaultTimeout)
	writeJSON(w, 200, map[string]any{"ok": res.Status == ping.StatusOK, "status": res.Status, "rtt_ms": res.RTTms,
		"output": pingText(res), "elapsed_ms": time.Since(start).Milliseconds()})
}

// pingText は1台版の画面表示用の要約文。
func pingText(r ping.Result) string {
	switch r.Status {
	case ping.StatusOK:
		return fmt.Sprintf("%s から応答: %.0f ms", r.IP, r.RTTms)
	case ping.StatusTimeout:
		return r.IP + " から応答がありません（経路がない・ICMPを遮断している場合も含みます）"
	case ping.StatusUnreachable:
		return r.IP + " へ到達できません " + r.Detail
	}
	return r.IP + " へPingを送れません: " + r.Detail
}

// pingAllowed は手入力なしで打ってよいIP（登録スイッチ・現在見えている機器・登録済み機器の最終IP）。
func (s *Server) pingAllowed() map[string]bool {
	ok := map[string]bool{}
	// 登録スイッチは未接続（状態未取得）でも打てるよう、設定のIPも含める
	if cfgs, err := s.Store.Switches(s.Engine.ProfileID()); err == nil {
		for _, c := range cfgs {
			ok[c.IP] = true
		}
	}
	for _, st := range s.Engine.States() {
		if st.IP != "" {
			ok[st.IP] = true
		}
	}
	if rows, err := s.Engine.Inventory(engine.InventoryOptions{}); err == nil {
		for _, r := range rows {
			if r.IP != "" {
				ok[r.IP] = true
			}
		}
	}
	return ok
}

// maxBulkPing は一括Pingの1回あたり宛先上限。手入力の範囲指定で大量送信しないため。
const maxBulkPing = 256

// pingBulk は複数宛先へ一括でPingする。ips は監視中に分かっているIPのみ、manual は任意の
// IP・範囲（a.b.c.d-e / a.b.c.d-a.b.c.e）・CIDR をカンマ/空白区切りで受け付ける（ロック中は不可）。
func (s *Server) pingBulk(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IPs    []string `json:"ips"`
		Manual string   `json:"manual"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "一括Pingできません", err.Error(), "")
		return
	}
	allowed := s.pingAllowed()
	seen := map[string]bool{}
	var targets []string
	source := map[string]string{}
	for _, ip := range in.IPs {
		ip = strings.TrimSpace(ip)
		if seen[ip] {
			continue
		}
		if !allowed[ip] {
			writeErr(w, 403, "一括Pingできません", ip+" は監視中に見つかったIPではありません", "任意のIPは手入力欄に入れてください")
			return
		}
		seen[ip] = true
		targets = append(targets, ip)
		source[ip] = "known"
	}
	if m := strings.TrimSpace(in.Manual); m != "" {
		if s.isLocked() {
			writeErr(w, http.StatusLocked, "閲覧専用ロック中は手入力のPingはできません", "", "一覧の機器へのPingは実行できます。手入力はロック解除後に行ってください")
			return
		}
		for _, part := range strings.FieldsFunc(m, func(c rune) bool { return strings.ContainsRune(", \n\t、", c) }) {
			ips, err := expandRange(part)
			if err != nil {
				writeErr(w, 400, "手入力のIPを解釈できません", part+": "+err.Error(), "例: 192.168.1.10、192.168.1.10-50、192.168.1.0/24")
				return
			}
			for _, ip := range ips {
				if !seen[ip] {
					seen[ip] = true
					targets = append(targets, ip)
					source[ip] = "manual"
				}
				if len(targets) > maxBulkPing {
					writeErr(w, 400, "宛先が多すぎます", fmt.Sprintf("1回の上限は%d件です", maxBulkPing), "範囲を分けて実行してください")
					return
				}
			}
		}
	}
	if len(targets) == 0 {
		writeErr(w, 400, "Pingの宛先がありません", "", "機器を選ぶか、IPを手入力してください")
		return
	}
	if len(targets) > maxBulkPing {
		writeErr(w, 400, "宛先が多すぎます", fmt.Sprintf("1回の上限は%d件です", maxBulkPing), "絞り込んでから実行してください")
		return
	}
	start := time.Now()
	res := ping.Many(r.Context(), targets, ping.DefaultTimeout)
	out := make([]map[string]any, len(res))
	okN := 0
	for i, x := range res {
		if x.Status == ping.StatusOK {
			okN++
		}
		out[i] = map[string]any{"ip": x.IP, "status": x.Status, "rtt_ms": x.RTTms, "detail": x.Detail, "source": source[x.IP]}
	}
	writeJSON(w, 200, map[string]any{"results": out, "ok": okN, "total": len(res), "elapsed_ms": time.Since(start).Milliseconds(), "at": time.Now().UTC()})
}

// ---- イベント ----

func (s *Server) eventFilter(r *http.Request) store.EventFilter {
	q := r.URL.Query()
	f := store.EventFilter{Category: q.Get("category"), Severity: q.Get("severity"), Q: q.Get("q")}
	f.SwitchID, _ = strconv.ParseInt(q.Get("switch_id"), 10, 64)
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if v := q.Get("from"); v != "" {
		f.From, _ = time.Parse(time.RFC3339, v)
	}
	if v := q.Get("to"); v != "" {
		f.To, _ = time.Parse(time.RFC3339, v)
	}
	return f
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	evs, err := s.Store.Events(s.eventFilter(r))
	if err != nil {
		writeErr(w, 500, "イベントを取得できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, evs)
}

func (s *Server) ack(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	if err := s.Store.Ack(id); err != nil {
		writeErr(w, 500, "確認済みにできません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"acked": id})
}

func (s *Server) ackAll(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.AckAll(); err != nil {
		writeErr(w, 500, "確認済みにできません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"acked": "all"})
}

func (s *Server) marker(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	readJSON(r, &in)
	note := strings.TrimSpace(in.Note)
	if note == "" {
		note = "インシデント・マーカー"
	}
	if len([]rune(note)) > 200 {
		note = string([]rune(note)[:200])
	}
	ev := model.Event{TS: time.Now().UTC(), Category: "MARKER", Severity: model.SevInfo, Code: "MARKER", Title: "★ " + note}
	s.Engine.OnEvent(ev)
	evs, _ := s.Store.Events(store.EventFilter{Category: "MARKER", Limit: 1})
	if len(evs) > 0 {
		writeJSON(w, 200, evs[0])
		return
	}
	writeJSON(w, 200, ev)
}

// ---- ベースライン ----

func (s *Server) baselines(w http.ResponseWriter, r *http.Request) {
	bs, err := s.Store.Baselines(s.Engine.ProfileID())
	if err != nil {
		writeErr(w, 500, "ベースライン一覧を取得できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, bs)
}

func (s *Server) createBaseline(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	readJSON(r, &in)
	if strings.TrimSpace(in.Name) == "" {
		in.Name = "ベースライン " + time.Now().Format("01/02 15:04")
	}
	states := s.Engine.States()
	id, err := s.Store.SaveBaseline(s.Engine.ProfileID(), in.Name, states)
	if err != nil {
		writeErr(w, 500, "ベースラインを保存できません", err.Error(), "")
		return
	}
	s.Engine.SetSFPBaseline(states)
	s.Engine.OnEvent(model.Event{Category: "BASELINE", Severity: model.SevInfo, Code: "BASELINE_SAVED", Title: "ベースライン保存: " + in.Name})
	writeJSON(w, 200, map[string]any{"id": id, "name": in.Name})
}

func (s *Server) baselineDiff(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	b, err := s.Store.Baseline(id)
	if err != nil {
		writeErr(w, 404, "ベースラインが見つかりません", err.Error(), "")
		return
	}
	s.Engine.SetSFPBaseline(b)
	writeJSON(w, 200, map[string]any{"diffs": s.Engine.BaselineDiff(b)})
}

func (s *Server) deleteBaseline(w http.ResponseWriter, r *http.Request) {
	id, _ := pathID(r, "id")
	if err := s.Store.DeleteBaseline(id); err != nil {
		writeErr(w, 500, "削除できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": id})
}

// ---- 書出・読込 ----

func (s *Server) exportPack(w http.ResponseWriter, r *http.Request) {
	var in struct {
		From     time.Time `json:"from"`
		To       time.Time `json:"to"`
		MarkerID int64     `json:"marker_id"`
		Minutes  int       `json:"minutes"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "書出条件が不正です", err.Error(), "")
		return
	}
	if in.MarkerID > 0 {
		evs, _ := s.Store.Events(store.EventFilter{Category: "MARKER", Limit: 10000})
		found := false
		for _, e := range evs {
			if e.ID == in.MarkerID {
				m := in.Minutes
				if m <= 0 {
					m = 5
				}
				in.From, in.To, found = e.TS.Add(-time.Duration(m)*time.Minute), e.TS.Add(time.Duration(m)*time.Minute), true
			}
		}
		if !found {
			writeErr(w, 404, "マーカーが見つかりません", "", "")
			return
		}
	}
	if in.To.IsZero() {
		in.To = time.Now().UTC()
	}
	if in.From.IsZero() {
		in.From = in.To.Add(-time.Hour)
	}
	if !in.From.Before(in.To) {
		writeErr(w, 400, "書出期間が不正です", "開始が終了より後です", "")
		return
	}
	name := fmt.Sprintf("avline-%s.avsight", in.From.Local().Format("20060102-1504"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	if err := s.Store.ExportPack(w, s.tmpDir(), in.From, in.To, s.Version); err != nil {
		// ヘッダ送信後の可能性があるためログにも残す
		writeErr(w, 500, "書出に失敗しました", err.Error(), "ディスク空き容量を確認してください")
	}
}

func (s *Server) exportTable(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := timeRange(r, 24*time.Hour)
	var rows []map[string]any
	var cols []string
	switch q.Get("type") {
	case "events", "":
		evs, _ := s.Store.Events(store.EventFilter{From: from, To: to, Limit: 10000})
		cols = []string{"ts", "switch_name", "if_index", "category", "severity", "code", "title", "hint", "acked"}
		for _, e := range evs {
			rows = append(rows, map[string]any{"ts": e.TS.Local().Format("2006-01-02 15:04:05"), "switch_name": e.SwitchName, "if_index": e.IfIndex,
				"category": e.Category, "severity": e.Severity, "code": e.Code, "title": e.Title, "hint": e.Hint, "acked": e.Acked})
		}
	case "connections":
		rows, _ = s.Store.Connections(from, to)
		cols = []string{"switch_id", "if_index", "mac", "vendor", "ip", "vlan", "lldp_name", "first_seen", "last_seen", "active"}
	case "ports":
		cols = []string{"switch_name", "port", "oper", "speed_mbps", "rx_bps", "tx_bps", "err_since_reset", "discard_since_reset", "poe_w", "sfp_rx_dbm", "devices"}
		for _, st := range s.Engine.States() {
			for _, p := range st.Ports {
				var ds []string
				for _, d := range p.Devices {
					ds = append(ds, strings.TrimSpace(d.MAC+" "+d.IP+" "+firstNonEmpty(d.Label, d.LLDPName, d.Vendor)))
				}
				row := map[string]any{"switch_name": st.Name, "port": p.Name, "oper": p.Oper, "speed_mbps": p.SpeedMbps, "rx_bps": int64(p.RxBps), "tx_bps": int64(p.TxBps),
					"err_since_reset": p.ErrSinceReset, "discard_since_reset": p.DiscardSinceReset, "devices": strings.Join(ds, " / ")}
				if p.PoE != nil {
					row["poe_w"] = p.PoE.PowerW
				}
				if p.SFP != nil {
					row["sfp_rx_dbm"] = p.SFP.RxDBm
				}
				rows = append(rows, row)
			}
		}
	default:
		writeErr(w, 400, "未対応の種別です", "", "events / connections / ports のいずれか")
		return
	}
	if strings.HasSuffix(r.URL.Path, "/json") {
		w.Header().Set("Content-Disposition", `attachment; filename="`+q.Get("type")+`.json"`)
		if rows == nil {
			rows = []map[string]any{}
		}
		writeJSON(w, 200, rows)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+firstNonEmpty(q.Get("type"), "events")+`.csv"`)
	if q.Get("bom") == "1" {
		w.Write([]byte{0xEF, 0xBB, 0xBF}) // Excel向けBOM
	}
	cw := csv.NewWriter(w)
	cw.Write(cols)
	for _, row := range rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			if v, ok := row[c]; ok && v != nil {
				rec[i] = csvSafe(fmt.Sprint(v))
			}
		}
		cw.Write(rec)
	}
	cw.Flush()
}

// csvSafe は表計算ソフトの数式インジェクションを防ぐ。
func csvSafe(v string) string {
	if v != "" && strings.ContainsRune("=+-@", rune(v[0])) {
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			return "'" + v
		}
	}
	return v
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

func randomID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) replayImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<30)
	f, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, "ファイルを受け取れません", err.Error(), ".avsight ファイルを指定してください")
		return
	}
	defer f.Close()
	sid := randomID()
	dir := filepath.Join(s.tmpDir(), "replay-"+sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeErr(w, 500, "作業領域を作成できません", err.Error(), "")
		return
	}
	zp := filepath.Join(dir, "in.avsight")
	out, err := os.Create(zp)
	if err == nil {
		_, err = io.Copy(out, f)
		out.Close()
	}
	if err != nil {
		os.RemoveAll(dir)
		writeErr(w, 500, "ファイルを保存できません", err.Error(), "ディスク空き容量を確認してください")
		return
	}
	st, man, err := store.ImportPack(zp, dir)
	if err != nil {
		os.RemoveAll(dir)
		writeErr(w, 400, "ログパックを読み込めません", err.Error(), "")
		return
	}
	s.mu.Lock()
	// 古いセッションを掃除（同時保持は3件まで）
	if len(s.replays) >= 3 {
		var oldest string
		for k, v := range s.replays {
			if oldest == "" || v.used.Before(s.replays[oldest].used) {
				oldest = k
			}
		}
		s.closeReplay(oldest)
	}
	s.replays[sid] = &replay{st: st, man: man, dir: dir, used: time.Now()}
	s.mu.Unlock()
	writeJSON(w, 200, map[string]any{"session_id": sid, "from": man.FromTS, "to": man.ToTS, "switches": man.Switches})
}

func (s *Server) closeReplay(sid string) {
	if rp := s.replays[sid]; rp != nil {
		rp.st.Close()
		os.RemoveAll(rp.dir)
		delete(s.replays, sid)
	}
}

func (s *Server) getReplay(w http.ResponseWriter, r *http.Request) *replay {
	s.mu.Lock()
	defer s.mu.Unlock()
	rp := s.replays[r.PathValue("sid")]
	if rp == nil {
		writeErr(w, 404, "再生セッションが見つかりません", "期限切れまたは閉じられています", "もう一度ファイルを読み込んでください")
		return nil
	}
	rp.used = time.Now()
	return rp
}

func (s *Server) replayTimeline(w http.ResponseWriter, r *http.Request) {
	rp := s.getReplay(w, r)
	if rp == nil {
		return
	}
	from, to := rp.man.FromTS, rp.man.ToTS
	if a, b := rp.st.TimeRange(); !a.IsZero() && a.Unix() > 0 {
		if a.Before(from) || from.IsZero() {
			from = a
		}
		if b.After(to) {
			to = b
		}
	}
	evs, _ := rp.st.Events(store.EventFilter{Limit: 5000})
	var marks []map[string]any
	for _, e := range evs {
		if e.Category == "MARKER" || e.Severity == model.SevCritical || e.Category == "TRAP" || e.Severity == model.SevWarning {
			marks = append(marks, map[string]any{"ts": e.TS, "severity": e.Severity, "title": e.Title, "category": e.Category})
		}
	}
	sort.Slice(marks, func(i, j int) bool { return marks[i]["ts"].(time.Time).Before(marks[j]["ts"].(time.Time)) })
	if marks == nil {
		marks = []map[string]any{}
	}
	writeJSON(w, 200, map[string]any{"from": from, "to": to, "markers": marks})
}

func (s *Server) replaySnapshot(w http.ResponseWriter, r *http.Request) {
	rp := s.getReplay(w, r)
	if rp == nil {
		return
	}
	ts, err := time.Parse(time.RFC3339, r.URL.Query().Get("ts"))
	if err != nil {
		ts = time.Now()
	}
	at, states, err := rp.st.SnapshotAt(ts)
	if err != nil {
		writeErr(w, 404, "スナップショットがありません", err.Error(), "")
		return
	}
	writeJSON(w, 200, map[string]any{"ts": at, "switches": states})
}

func (s *Server) replayEvents(w http.ResponseWriter, r *http.Request) {
	rp := s.getReplay(w, r)
	if rp == nil {
		return
	}
	evs, err := rp.st.Events(s.eventFilter(r))
	if err != nil {
		writeErr(w, 500, "イベントを取得できません", err.Error(), "")
		return
	}
	writeJSON(w, 200, evs)
}

func (s *Server) replayDelete(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.closeReplay(r.PathValue("sid"))
	s.mu.Unlock()
	writeJSON(w, 200, map[string]string{"closed": r.PathValue("sid")})
}

// ---- 探索 ----

func (s *Server) discoveryScan(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CIDR      string `json:"cidr"`
		Community string `json:"community"`
		Port      int    `json:"port"`
		PortTo    int    `json:"port_to"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "探索条件が不正です", err.Error(), "")
		return
	}
	ips, err := expandRange(in.CIDR)
	if err != nil {
		writeErr(w, 400, "探索範囲が不正です", err.Error(), "例: 192.168.0.0/24 または 192.168.0.10-192.168.0.50（最大1024アドレス）")
		return
	}
	if in.Community == "" {
		in.Community = "public"
	}
	if in.Port == 0 {
		in.Port = 161
	}
	if in.PortTo < in.Port || in.PortTo-in.Port > 63 {
		in.PortTo = in.Port // 複数ポート走査はラボ用（同一IPに複数エージェント）
	}
	registered := map[string]bool{}
	sws, _ := s.Store.Switches(s.Engine.ProfileID())
	for _, c := range sws {
		registered[fmt.Sprintf("%s:%d", c.IP, c.SNMPPort)] = true
	}
	type target struct {
		ip   string
		port int
	}
	jobs := make(chan target)
	var mu sync.Mutex
	found := []map[string]any{}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				c, err := sx.Dial(t.ip, uint16(t.port), in.Community, 700*time.Millisecond, 0)
				if err != nil {
					continue
				}
				res, err := c.Get([]string{mib.SysObjectID, mib.SysName, mib.SysDescr})
				c.Close()
				if err != nil {
					continue
				}
				oid, _ := res[mib.SysObjectID].Value.(string)
				oid = strings.TrimPrefix(oid, ".")
				if !strings.HasPrefix(oid, mib.NetgearEnterprise+".") {
					continue // NETGEAR以外は対象外
				}
				mu.Lock()
				found = append(found, map[string]any{"ip": t.ip, "port": t.port, "sys_name": sx.Str(res[mib.SysName]), "sys_descr": sx.Str(res[mib.SysDescr]),
					"sys_object_id": oid, "registered": registered[fmt.Sprintf("%s:%d", t.ip, t.port)]})
				mu.Unlock()
			}
		}()
	}
	for _, ip := range ips {
		for p := in.Port; p <= in.PortTo; p++ {
			select {
			case jobs <- target{ip, p}:
			case <-r.Context().Done():
			}
		}
	}
	close(jobs)
	wg.Wait()
	sort.Slice(found, func(i, j int) bool {
		return fmt.Sprint(found[i]["ip"], found[i]["port"]) < fmt.Sprint(found[j]["ip"], found[j]["port"])
	})
	writeJSON(w, 200, map[string]any{"found": found})
}

// expandRange は CIDR または "a-b" 範囲をIPv4一覧にする（最大1024件）。
func expandRange(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	var start, end uint32
	if strings.Contains(s, "/") {
		_, n, err := net.ParseCIDR(s)
		if err != nil || n.IP.To4() == nil {
			return nil, fmt.Errorf("CIDRを解釈できません")
		}
		ones, bits := n.Mask.Size()
		start = ipU32(n.IP.To4())
		end = start + (1 << uint(bits-ones)) - 1
		if bits-ones >= 2 {
			start++
			end-- // ネットワーク/ブロードキャストアドレスを除外
		}
	} else if a, b, ok := strings.Cut(s, "-"); ok {
		ia, ib := net.ParseIP(strings.TrimSpace(a)).To4(), net.ParseIP(strings.TrimSpace(b)).To4()
		// 短縮形 192.168.1.10-50（末尾オクテットだけ指定）
		if n, err := strconv.Atoi(strings.TrimSpace(b)); err == nil && ia != nil && n >= 0 && n <= 255 {
			ib = net.IPv4(ia[0], ia[1], ia[2], byte(n)).To4()
		}
		if ia == nil || ib == nil {
			return nil, fmt.Errorf("範囲を解釈できません")
		}
		start, end = ipU32(ia), ipU32(ib)
	} else if ip := net.ParseIP(s).To4(); ip != nil {
		start, end = ipU32(ip), ipU32(ip)
	} else {
		return nil, fmt.Errorf("形式が不正です")
	}
	if end < start || end-start >= 1024 {
		return nil, fmt.Errorf("範囲が広すぎるか逆順です")
	}
	var out []string
	for i := uint64(0); i <= uint64(end-start); i++ {
		v := start + uint32(i)
		out = append(out, fmt.Sprintf("%d.%d.%d.%d", v>>24, (v>>16)&255, (v>>8)&255, v&255))
	}
	return out, nil
}

func ipU32(ip net.IP) uint32 {
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func (s *Server) lldpCandidates(w http.ResponseWriter, r *http.Request) {
	reg := map[string]bool{}
	sws, _ := s.Store.Switches(s.Engine.ProfileID())
	for _, c := range sws {
		reg[c.IP] = true
	}
	writeJSON(w, 200, s.Engine.LLDPCandidates(reg))
}

// ---- ラボ ----

func (s *Server) simScenarios(w http.ResponseWriter, r *http.Request) {
	if s.Lab == nil {
		http.NotFound(w, r)
		return
	}
	// docs/api.md の契約どおり配列で返す（switch_index はスイッチ名 AV-SW-NN の NN-1）
	writeJSON(w, 200, simScenarioList())
}

func (s *Server) simFault(w http.ResponseWriter, r *http.Request) {
	if s.Lab == nil {
		http.NotFound(w, r)
		return
	}
	var in struct {
		SwitchIndex int    `json:"switch_index"`
		Scenario    string `json:"scenario"`
	}
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "障害注入の指定が不正です", err.Error(), "")
		return
	}
	if err := s.Lab.Inject(in.SwitchIndex, in.Scenario); err != nil {
		writeErr(w, 400, "障害を注入できません", err.Error(), "")
		return
	}
	s.Engine.OnEvent(model.Event{Category: "SYSTEM", Severity: model.SevInfo, Code: "LAB_FAULT", Title: fmt.Sprintf("【ラボ】障害注入: %s → AV-SW-%02d", in.Scenario, in.SwitchIndex+1)})
	writeJSON(w, 200, map[string]any{"injected": in.Scenario})
}

func (s *Server) simReset(w http.ResponseWriter, r *http.Request) {
	if s.Lab == nil {
		http.NotFound(w, r)
		return
	}
	s.Lab.Reset()
	writeJSON(w, 200, map[string]string{"status": "reset"})
}
