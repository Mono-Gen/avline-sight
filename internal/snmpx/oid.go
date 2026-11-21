// Package snmpx は OID操作・インデックス復号・PortListビット解析などSNMP共通処理を提供する。
package snmpx

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParseOID は "1.3.6..."（先頭ドット可）を数値列へ変換する。
func ParseOID(s string) ([]uint32, error) {
	s = strings.TrimPrefix(s, ".")
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ".")
	out := make([]uint32, len(parts))
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("不正なOID %q: %w", s, err)
		}
		out[i] = uint32(v)
	}
	return out, nil
}

// MustOID はテスト・定数用。不正値はpanic（コード上の定数誤りを即座に検出するため）。
func MustOID(s string) []uint32 {
	o, err := ParseOID(s)
	if err != nil {
		panic(err)
	}
	return o
}

// FormatOID は数値列を "1.3.6..." 表記にする。
func FormatOID(o []uint32) string {
	var b strings.Builder
	for i, v := range o {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(strconv.FormatUint(uint64(v), 10))
	}
	return b.String()
}

// CompareOID は辞書式比較（a<b:-1, a==b:0, a>b:1）。GETNEXTの順序に使う。
func CompareOID(a, b []uint32) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// Suffix は oid が prefix 配下なら残りのインデックス部を返す。
func Suffix(oid, prefix string) ([]uint32, bool) {
	oid = strings.TrimPrefix(oid, ".")
	prefix = strings.TrimPrefix(prefix, ".")
	if !strings.HasPrefix(oid, prefix+".") {
		return nil, false
	}
	idx, err := ParseOID(oid[len(prefix)+1:])
	if err != nil {
		return nil, false
	}
	return idx, true
}

// IndexMAC はインデックス中の6オクテットをMAC表記にする。
func IndexMAC(idx []uint32) string {
	if len(idx) < 6 {
		return ""
	}
	b := make(net.HardwareAddr, 6)
	for i := 0; i < 6; i++ {
		b[i] = byte(idx[i])
	}
	return strings.ToUpper(b.String())
}

// IndexIPv4 は4オクテットをIPv4表記にする。
func IndexIPv4(idx []uint32) string {
	if len(idx) < 4 {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", idx[0], idx[1], idx[2], idx[3])
}

// DecodeInetIndex は InetAddressType + 長さ前置 InetAddress のインデックスを復号する。
// 戻り値: アドレス文字列, 消費した要素数。IPv4(1)/IPv6(2)以外は16進表記。
func DecodeInetIndex(idx []uint32) (string, int) {
	if len(idx) < 2 {
		return "", 0
	}
	atype, l := idx[0], int(idx[1])
	if len(idx) < 2+l {
		return "", 0
	}
	raw := make([]byte, l)
	for i := 0; i < l; i++ {
		raw[i] = byte(idx[2+i])
	}
	switch {
	case atype == 1 && l == 4, atype == 2 && l == 16:
		return net.IP(raw).String(), 2 + l
	}
	return fmt.Sprintf("%x", raw), 2 + l
}

// BytesToIP は InetAddress 値（OCTET STRING）をIP表記にする。長さ0は空文字。
func BytesToIP(b []byte) string {
	if len(b) == 4 || len(b) == 16 {
		return net.IP(b).String()
	}
	return ""
}

// BytesToMAC は6バイトの値をMAC表記にする。
func BytesToMAC(b []byte) string {
	if len(b) != 6 {
		return ""
	}
	return strings.ToUpper(net.HardwareAddr(b).String())
}

// PortListBits は PortList / AgentPortMask を解析し、立っているポート番号（1始まり）を返す。
// 仕様: 固定長前提を禁止し任意バイト長を動的に解析する（M4250H 48ポート級対応）。
// 最上位ビットがポート1（RFC 2674 PortList の定義）。
func PortListBits(b []byte) []int {
	var ports []int
	for i, by := range b {
		for bit := 0; bit < 8; bit++ {
			if by&(0x80>>bit) != 0 {
				ports = append(ports, i*8+bit+1)
			}
		}
	}
	return ports
}

// MakePortList は PortListBits の逆変換（シミュレータ用）。
func MakePortList(ports []int, nbytes int) []byte {
	b := make([]byte, nbytes)
	for _, p := range ports {
		if p < 1 || (p-1)/8 >= nbytes {
			continue
		}
		b[(p-1)/8] |= 0x80 >> ((p - 1) % 8)
	}
	return b
}
