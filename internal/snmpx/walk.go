package snmpx

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

// Client はスイッチ1台分のSNMPv2c接続。1つのgoroutineからのみ使う前提（gosnmpは並行安全でない）。
type Client struct {
	g *gosnmp.GoSNMP
	// MaxRepetitions は1回のGETBULKで返させる行数の目安（48ポート級でtooBigを避けるため既定20）。
	MaxRepetitions int
}

// Dial はUDPソケットを開く（SNMPはコネクションレスのため到達確認ではない）。
func Dial(host string, port uint16, community string, timeout time.Duration, retries int) (*Client, error) {
	g := &gosnmp.GoSNMP{Target: host, Port: port, Community: community, Version: gosnmp.Version2c,
		Timeout: timeout, Retries: retries, MaxOids: 60}
	if err := g.Connect(); err != nil {
		return nil, fmt.Errorf("SNMP接続準備に失敗 %s:%d: %w", host, port, err)
	}
	return &Client{g: g, MaxRepetitions: 20}, nil
}

// Close はソケットを閉じる。
func (c *Client) Close() error {
	if c.g.Conn != nil {
		return c.g.Conn.Close()
	}
	return nil
}

// Get はスカラーを取得する。存在しないOIDは結果に含めない。
func (c *Client) Get(oids []string) (map[string]gosnmp.SnmpPDU, error) {
	out := map[string]gosnmp.SnmpPDU{}
	for i := 0; i < len(oids); i += 30 {
		j := i + 30
		if j > len(oids) {
			j = len(oids)
		}
		r, err := c.g.Get(oids[i:j])
		if err != nil {
			return nil, err
		}
		for _, v := range r.Variables {
			if v.Type == gosnmp.NoSuchObject || v.Type == gosnmp.NoSuchInstance || v.Type == gosnmp.EndOfMibView {
				continue
			}
			out[strings.TrimPrefix(v.Name, ".")] = v
		}
	}
	return out, nil
}

// Row はテーブル1行分（列OID→値）。Index はインデックス部（"1.2.3"）。
type Row struct {
	Index []uint32
	Cols  map[string]gosnmp.SnmpPDU
}

// WalkColumns は複数列を同時にGETBULKで走査し、インデックス毎の行にまとめる。
// 列ごとに個別WALKするより往復回数が少なく、スイッチCPU負荷を抑えられる。
func (c *Client) WalkColumns(cols []string) ([]Row, error) {
	type cur struct {
		col  string
		next string
		done bool
	}
	curs := make([]*cur, len(cols))
	for i, col := range cols {
		curs[i] = &cur{col: col, next: col}
	}
	rows := map[string]*Row{}
	var order []string
	for iter := 0; iter < 10000; iter++ {
		var active []*cur
		for _, c := range curs {
			if !c.done {
				active = append(active, c)
			}
		}
		if len(active) == 0 {
			break
		}
		// 1リクエストのVarBind数を抑える
		if len(active) > 12 {
			active = active[:12]
		}
		reps := c.MaxRepetitions
		if per := 48 / len(active); per < reps {
			reps = per
		}
		if reps < 1 {
			reps = 1
		}
		names := make([]string, len(active))
		for i, a := range active {
			names[i] = a.next
		}
		r, err := c.g.GetBulk(names, 0, uint32(reps))
		if err != nil {
			return nil, err
		}
		if len(r.Variables) == 0 {
			for _, a := range active {
				a.done = true
			}
			continue
		}
		progressed := make([]bool, len(active))
		for k, v := range r.Variables {
			a := active[k%len(active)]
			if a.done {
				continue
			}
			name := strings.TrimPrefix(v.Name, ".")
			idx, ok := Suffix(name, a.col)
			if !ok || v.Type == gosnmp.EndOfMibView || v.Type == gosnmp.NoSuchObject {
				a.done = true
				continue
			}
			key := FormatOID(idx)
			row := rows[key]
			if row == nil {
				row = &Row{Index: idx, Cols: map[string]gosnmp.SnmpPDU{}}
				rows[key] = row
				order = append(order, key)
			}
			row.Cols[a.col] = v
			if name != a.next {
				a.next = name
				progressed[k%len(active)] = true
			}
		}
		for i, a := range active {
			if !progressed[i] {
				a.done = true // 進まない＝応答不正。無限ループ防止
			}
		}
	}
	out := make([]Row, 0, len(order))
	for _, k := range order {
		out = append(out, *rows[k])
	}
	return out, nil
}

// ---- 値変換ヘルパー ----

// Int は数値系PDUをint64へ。非数値は0。
func Int(p gosnmp.SnmpPDU) int64 {
	switch p.Type {
	case gosnmp.OctetString, gosnmp.ObjectIdentifier, gosnmp.IPAddress, gosnmp.Null, gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.EndOfMibView:
		return 0
	}
	if p.Value == nil {
		return 0
	}
	return gosnmp.ToBigInt(p.Value).Int64()
}

// U64 はCounter64を含む非負値をuint64へ。
func U64(p gosnmp.SnmpPDU) uint64 {
	if p.Value == nil {
		return 0
	}
	switch p.Type {
	case gosnmp.Counter64, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Uinteger32, gosnmp.Integer:
		b := gosnmp.ToBigInt(p.Value)
		if b.Sign() < 0 {
			return 0
		}
		if b.IsUint64() {
			return b.Uint64()
		}
		return new(big.Int).And(b, new(big.Int).SetUint64(^uint64(0))).Uint64()
	}
	return 0
}

// Str はOCTET STRINGを文字列化（制御文字は除去）。
func Str(p gosnmp.SnmpPDU) string {
	switch v := p.Value.(type) {
	case []byte:
		return strings.TrimRight(strings.Map(func(r rune) rune {
			if r < 0x20 && r != '\t' {
				return -1
			}
			return r
		}, string(v)), " ")
	case string:
		return v
	}
	return ""
}

// Bytes はOCTET STRINGの生バイト。
func Bytes(p gosnmp.SnmpPDU) []byte {
	if b, ok := p.Value.([]byte); ok {
		return b
	}
	return nil
}

// CounterDelta はカウンタの増分を返す。巻き戻り（再起動・32bitラップ）時は ok=false。
// 32bitカウンタのラップアラウンドは補正し、それ以上の減少は「リセット」と判定する。
func CounterDelta(prev, cur uint64, is32 bool) (uint64, bool) {
	if cur >= prev {
		return cur - prev, true
	}
	if is32 && prev <= 0xffffffff {
		d := (0xffffffff - prev) + cur + 1
		// ラップとみなすのは差が小さい（レート的に妥当な）場合のみ
		if d < 1<<30 {
			return d, true
		}
	}
	return 0, false
}
