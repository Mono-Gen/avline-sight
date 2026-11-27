package sim

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/snmpx"
)

// Replay は実機の SNMP ダンプ（1行1値 "OID = 値"）を読み、その内容のまま応答する仮想エージェント群。
// 実機の無いPCで、取得した実機の状態を相手にアプリを動かすために使う。
type Replay struct {
	Agents []*ReplayAgent
}

// ReplayAgent はダンプ1台分。
type ReplayAgent struct {
	Index    int
	SourceIP string // ダンプ元の実機IP（表示名に使う）
	mu       sync.Mutex
	tree     []entry
	upIdx    int       // sysUpTime の位置（無ければ -1）
	upBase   uint32    // ダンプ時点の sysUpTime
	started  time.Time // 再生開始時刻（sysUpTime を進める基準）
}

// ダンプのファイル名: snmp_<ip>.txt / snmp_mc_<ip>.txt / snmp_std_<ip>.txt 等。末尾の IPv4 で台を分ける。
var dumpName = regexp.MustCompile(`^snmp_(?:[A-Za-z0-9]+_)?(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\.txt$`)

// LoadReplay は dir 内のダンプを IP ごとにまとめて読み込む。同じ OID が複数ファイルにあれば先に読んだ方を使う。
func LoadReplay(dir string) (*Replay, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("ダンプのフォルダを開けません %s: %w", dir, err)
	}
	files := map[string][]string{}
	for _, e := range ents {
		if m := dumpName.FindStringSubmatch(e.Name()); m != nil && !e.IsDir() {
			files[m[1]] = append(files[m[1]], filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s に snmp_<IP>.txt 形式のダンプがありません", dir)
	}
	ips := make([]string, 0, len(files))
	for ip := range files {
		ips = append(ips, ip)
	}
	sort.Slice(ips, func(i, j int) bool { return ipLess(ips[i], ips[j]) })
	r := &Replay{}
	for i, ip := range ips {
		sort.Strings(files[ip])
		tree := map[string]entry{}
		for _, f := range files[ip] {
			if err := readDump(f, tree); err != nil {
				return nil, err
			}
		}
		a := &ReplayAgent{Index: i, SourceIP: ip, upIdx: -1}
		for _, e := range tree {
			a.tree = append(a.tree, e)
		}
		sort.Slice(a.tree, func(i, j int) bool { return snmpx.CompareOID(a.tree[i].oid, a.tree[j].oid) < 0 })
		up := snmpx.MustOID(mib.SysUpTime)
		for k, e := range a.tree {
			if snmpx.CompareOID(e.oid, up) == 0 {
				a.upIdx = k
				a.upBase = uint32(gosnmp.ToBigInt(e.pdu.Value).Uint64())
			}
		}
		r.Agents = append(r.Agents, a)
	}
	return r, nil
}

func ipLess(a, b string) bool {
	pa, pb := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	for i := 0; i < 4 && pa != nil && pb != nil; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return a < b
}

func readDump(path string, out map[string]entry) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("ダンプを開けません %s: %w", path, err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	line := 0
	for sc.Scan() {
		line++
		name, val, ok := strings.Cut(sc.Text(), " = ")
		if !ok {
			continue // ツールの見出し行など
		}
		name = strings.TrimSpace(name)
		if _, dup := out[name]; dup {
			continue
		}
		o, err := snmpx.ParseOID(name)
		if err != nil {
			return fmt.Errorf("%s %d行目: OID を読めません: %w", path, line, err)
		}
		t, v := inferValue(name, val)
		out[name] = entry{o, gosnmp.SnmpPDU{Name: "." + snmpx.FormatOID(o), Type: t, Value: v}}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("ダンプの読み込みに失敗 %s: %w", path, err)
	}
	return nil
}

// 数値の型が残っていないため、アプリが型を見て扱いを変える列だけ型を決める。それ以外の数値は Integer。
var typedColumns = []struct {
	prefix string
	t      gosnmp.Asn1BER
}{
	{mib.SysUpTime, gosnmp.TimeTicks},
	{mib.IfLastChange + ".", gosnmp.TimeTicks},
	{"1.3.6.1.2.1.2.2.1.5.", gosnmp.Gauge32}, // ifSpeed
	{"1.3.6.1.2.1.2.2.1.10.", gosnmp.Counter32},
	{"1.3.6.1.2.1.2.2.1.11.", gosnmp.Counter32},
	{"1.3.6.1.2.1.2.2.1.12.", gosnmp.Counter32},
	{mib.IfInDiscards + ".", gosnmp.Counter32},
	{mib.IfInErrors + ".", gosnmp.Counter32},
	{"1.3.6.1.2.1.2.2.1.15.", gosnmp.Counter32},
	{"1.3.6.1.2.1.2.2.1.16.", gosnmp.Counter32},
	{"1.3.6.1.2.1.2.2.1.17.", gosnmp.Counter32},
	{"1.3.6.1.2.1.2.2.1.18.", gosnmp.Counter32},
	{mib.IfOutDiscards + ".", gosnmp.Counter32},
	{mib.IfOutErrors + ".", gosnmp.Counter32},
	{"1.3.6.1.2.1.31.1.1.1.2.", gosnmp.Counter32}, // ifInMulticastPkts
	{"1.3.6.1.2.1.31.1.1.1.3.", gosnmp.Counter32},
	{"1.3.6.1.2.1.31.1.1.1.4.", gosnmp.Counter32},
	{"1.3.6.1.2.1.31.1.1.1.5.", gosnmp.Counter32},
	{"1.3.6.1.2.1.31.1.1.1.6.", gosnmp.Counter64}, // ifHC*（.6〜.13）
	{"1.3.6.1.2.1.31.1.1.1.7.", gosnmp.Counter64},
	{"1.3.6.1.2.1.31.1.1.1.8.", gosnmp.Counter64},
	{"1.3.6.1.2.1.31.1.1.1.9.", gosnmp.Counter64},
	{"1.3.6.1.2.1.31.1.1.1.10.", gosnmp.Counter64},
	{"1.3.6.1.2.1.31.1.1.1.11.", gosnmp.Counter64},
	{"1.3.6.1.2.1.31.1.1.1.12.", gosnmp.Counter64},
	{"1.3.6.1.2.1.31.1.1.1.13.", gosnmp.Counter64},
	{"1.3.6.1.2.1.31.1.1.1.15.", gosnmp.Gauge32}, // ifHighSpeed
	{"1.3.6.1.2.1.10.7.2.1.", gosnmp.Counter32},  // EtherLike dot3StatsTable
	{"1.3.6.1.2.1.10.7.10.1.", gosnmp.Counter32}, // dot3PauseTable
}

var dottedQuad = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)

// inferValue はダンプの値の書式から型と値を決める。
// 書式: OCTET STRING は `16進|"エスケープ"`、OID は先頭が "."、IpAddress は a.b.c.d、それ以外は数値。
func inferValue(oid, s string) (gosnmp.Asn1BER, any) {
	if h, _, ok := strings.Cut(s, "|"); ok && isHex(h) {
		b, err := hex.DecodeString(h)
		if err == nil {
			return gosnmp.OctetString, b
		}
	}
	if strings.HasPrefix(s, ".") {
		return gosnmp.ObjectIdentifier, s
	}
	if dottedQuad.MatchString(s) {
		return gosnmp.IPAddress, s
	}
	bare := strings.TrimPrefix(oid, ".")
	for _, c := range typedColumns {
		if strings.HasPrefix(bare, c.prefix) || bare == c.prefix {
			n, err := strconv.ParseUint(s, 10, 64)
			if err != nil {
				break
			}
			if c.t == gosnmp.Counter64 {
				return c.t, n
			}
			return c.t, uint32(n)
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return gosnmp.Integer, int(n)
	}
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		return gosnmp.Counter64, n
	}
	// 想定外の書式は文字列として返す（取り落とさない）
	return gosnmp.OctetString, []byte(s)
}

func isHex(s string) bool {
	if len(s)%2 != 0 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// snapshot は応答用のツリーを返す。sysUpTime だけ再生開始からの経過分を足す（巻き戻りで再起動と誤検知させない）。
func (a *ReplayAgent) snapshot(now time.Time) []entry {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.upIdx < 0 {
		return a.tree
	}
	tree := append([]entry(nil), a.tree...)
	e := tree[a.upIdx]
	e.pdu.Value = a.upBase + uint32(now.Sub(a.started)/(10*time.Millisecond))
	tree[a.upIdx] = e
	return tree
}

// Start は各台を 127.0.0.1:basePort+Index で待ち受ける。ctx 終了で止まる。
func (r *Replay) Start(ctx context.Context, basePort int) error {
	now := time.Now()
	for _, a := range r.Agents {
		a.started = now
		addr := fmt.Sprintf("127.0.0.1:%d", basePort+a.Index)
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			return fmt.Errorf("再生エージェント %s のUDP待受に失敗 (%s): %w", a.SourceIP, addr, err)
		}
		go func() { <-ctx.Done(); pc.Close() }()
		// コミュニティの照合は serve が行う（不一致は無応答）
		go serve(pc, func(req *gosnmp.SnmpPacket) *gosnmp.SnmpPacket {
			return respond(a.snapshot(time.Now()), req)
		})
	}
	return nil
}
