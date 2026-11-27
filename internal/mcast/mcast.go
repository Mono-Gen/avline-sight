// Package mcast はマルチキャストのグループ（IPv4 または L2 の MAC）から、推定IPと用途を引く。
//
// 出典（2026-11 調査）: IANA multicast-addresses、Audinate「Dante Information for Network Administrators」、
// Q-SYS Help「Multicast Traffic」「Multicast」、ESTA E1.31(sACN)、NDI white paper、OpenAvnu maap.h、
// Open-AVB ADP、IEEE 802.1 group address listing、NETGEAR KB 000064864。
//
// 01:00:5E の MAC は IPv4 の下位23ビットしか持たない（32個のIPが同じMACになる）。
// そのため MAC から引いた IP は「既知の用途に当てはまる候補」であり、確定ではない（Guessed=true）。
package mcast

import (
	"fmt"
	"net"
	"strings"
)

// Info は1グループの推定結果。
type Info struct {
	IP        string `json:"ip,omitempty"`         // 推定または実際のIPv4（L2専用プロトコルは空）
	Guessed   bool   `json:"ip_guessed,omitempty"` // MAC から推定した IP なら true
	Protocol  string `json:"protocol"`
	PurposeJa string `json:"purpose_ja,omitempty"`
	PurposeEn string `json:"purpose_en,omitempty"`
}

type entry struct {
	lo, hi   uint32
	protocol string
	ja, en   string
}

func ip4(s string) uint32 {
	b := net.ParseIP(s).To4()
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

func one(ip, proto, ja, en string) entry { v := ip4(ip); return entry{v, v, proto, ja, en} }
func span(lo, hi, proto, ja, en string) entry {
	return entry{ip4(lo), ip4(hi), proto, ja, en}
}

// table は優先順。1点のアドレスを先に、範囲を後に置く（MAC から引くとき、より具体的な一致を選ぶため）。
var table = []entry{
	one("224.0.0.1", "IGMP", "全ホスト宛て（IGMPクエリ）", "All hosts (IGMP queries)"),
	one("224.0.0.2", "IGMP", "全ルーター宛て", "All routers"),
	one("224.0.0.5", "OSPF", "OSPF 全ルーター", "OSPF all routers"),
	one("224.0.0.6", "OSPF", "OSPF DR", "OSPF designated routers"),
	one("224.0.0.9", "RIP", "RIPv2", "RIPv2"),
	one("224.0.0.13", "PIM", "PIM ルーター", "PIM routers"),
	one("224.0.0.18", "VRRP", "ルーター冗長化（VRRP）", "Router redundancy (VRRP)"),
	one("224.0.0.22", "IGMP", "IGMPv3 レポート", "IGMPv3 reports"),
	one("224.0.0.107", "PTP", "PTPv2 ピア遅延計測", "PTPv2 peer delay"),
	span("224.0.0.230", "224.0.0.233", "Dante", "Dante の制御・監視（機器探索・メータ）", "Dante control and monitoring"),
	one("224.0.0.251", "mDNS", "機器探索（mDNS: Dante・AES67 等）", "Device discovery (mDNS)"),
	one("224.0.0.252", "LLMNR", "名前解決（主に Windows PC）", "Name resolution (LLMNR, mostly Windows)"),
	one("224.0.1.1", "NTP", "時刻同期（NTP）", "Time sync (NTP)"),
	one("224.0.1.129", "PTP", "PTP クロック同期（Dante PTPv1・AES67/Q-SYS PTPv2）", "PTP clock sync (Dante PTPv1, AES67/Q-SYS PTPv2)"),
	span("224.0.1.130", "224.0.1.132", "PTP", "PTP クロック同期（Dante PTPv1 の別ドメイン）", "PTP clock sync (Dante PTPv1 other domains)"),
	one("224.0.23.175", "Q-SYS", "Q-SYS 機器探索（QDP）", "Q-SYS discovery (QDP)"),
	span("224.0.23.176", "224.0.23.185", "Q-SYS", "Q-SYS 周辺機器のファーム更新", "Q-SYS peripheral firmware update"),
	one("239.254.3.3", "Dante", "Dante PTP ログ（有効時のみ）", "Dante PTP logging"),
	one("239.255.255.250", "SSDP", "機器探索（SSDP/UPnP・WS-Discovery）", "Discovery (SSDP/UPnP, WS-Discovery)"),
	one("239.255.255.251", "Q-SYS", "Q-SYS カメラの探索（mDNS）", "Q-SYS camera discovery"),
	one("239.255.255.255", "SAP", "ストリーム告知（SAP: AES67）", "Stream announcement (SAP, AES67)"),
	span("239.69.0.0", "239.69.255.255", "AES67", "AES67 音声ストリーム", "AES67 audio streams"),
	span("233.254.0.0", "233.254.255.255", "Q-SYS", "Q-SYS AES67 送信", "Q-SYS AES67 transmit"),
	span("233.245.24.0", "233.245.31.255", "Q-SYS", "Q-SYS AES67 送信", "Q-SYS AES67 transmit"),
	span("233.252.0.0", "233.252.255.255", "Q-SYS", "Q-SYS 映像（NV シリーズ）", "Q-SYS video (NV series)"),
	span("233.229.24.0", "233.229.31.255", "Q-SYS", "Q-SYS 映像（NV シリーズ）", "Q-SYS video (NV series)"),
	span("233.253.0.0", "233.253.255.255", "Q-SYS", "Q-SYS PTZ カメラ映像", "Q-SYS PTZ camera video"),
	span("233.237.24.0", "233.237.31.255", "Q-SYS", "Q-SYS PTZ カメラ映像", "Q-SYS PTZ camera video"),
	span("239.255.0.0", "239.255.255.255", "Dante/sACN/NDI", "マルチキャスト音声・映像（Dante 既定範囲。sACN・NDI もこの範囲）", "Multicast audio/video (Dante default range; also sACN, NDI)"),
	span("239.0.0.0", "239.255.255.255", "AV", "組織内マルチキャスト（映像・音声: NVX・ST 2110・NDI 等）", "Organization-local multicast (AV streams)"),
	span("224.0.0.0", "224.0.0.255", "Link-local", "同一セグメント内の制御通信", "Link-local control"),
}

// L2 専用（IP ではない）マルチキャスト MAC。
var l2 = []struct {
	prefix, protocol, ja, en string
}{
	{"91:E0:F0:00:FF:00", "AVB MAAP", "AVB のアドレス取得（MAAP）", "AVB address acquisition (MAAP)"},
	{"91:E0:F0:01:00:00", "AVDECC", "AVB/Milan 機器探索（AVDECC）", "AVB/Milan discovery (AVDECC)"},
	{"91:E0:F0:00:", "AVB", "AVB ストリーム（MAAP で割当）", "AVB stream (MAAP pool)"},
	{"01:80:C2:00:00:0E", "LLDP/gPTP", "LLDP・gPTP（802.1AS）", "LLDP / gPTP (802.1AS)"},
	{"01:80:C2:00:00:21", "MVRP", "VLAN 登録（MVRP）", "VLAN registration (MVRP)"},
	{"01:80:C2:00:00:00", "STP", "スパニングツリー", "Spanning tree"},
	{"01:1B:19:00:00:00", "PTP", "PTPv2（L2 直接）", "PTPv2 over Ethernet"},
}

// Lookup はグループ文字列（"239.69.1.2" または "01:00:5E:45:01:02"）を引く。
func Lookup(group string) Info {
	g := strings.ToUpper(strings.TrimSpace(group))
	if ip := net.ParseIP(g).To4(); ip != nil {
		v := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
		if e, ok := match(v); ok {
			return withUniverse(Info{IP: ip.String(), Protocol: e.protocol, PurposeJa: e.ja, PurposeEn: e.en})
		}
		return Info{IP: ip.String(), Protocol: "Unknown"}
	}
	mac, err := net.ParseMAC(g)
	if err != nil || len(mac) != 6 {
		return Info{Protocol: "Unknown"}
	}
	if mac[0] == 0x01 && mac[1] == 0x00 && mac[2] == 0x5E {
		return fromMAC(mac)
	}
	for _, x := range l2 {
		if strings.HasPrefix(g, x.prefix) {
			return Info{Protocol: x.protocol, PurposeJa: x.ja, PurposeEn: x.en}
		}
	}
	return Info{Protocol: "L2"}
}

func match(v uint32) (entry, bool) {
	for _, e := range table {
		if v >= e.lo && v <= e.hi {
			return e, true
		}
	}
	return entry{}, false
}

// fromMAC は 32 個の候補 IP（224〜239 × 第2オクテット最上位ビットの有無）から、表の順で最初に当たるものを選ぶ。
func fromMAC(mac net.HardwareAddr) Info {
	low := uint32(mac[3]&0x7F)<<16 | uint32(mac[4])<<8 | uint32(mac[5])
	for _, e := range table {
		for first := uint32(224); first <= 239; first++ {
			for _, hi := range []uint32{0, 0x800000} {
				v := first<<24 | hi | low
				if v >= e.lo && v <= e.hi {
					return withUniverse(Info{IP: str(v), Guessed: true, Protocol: e.protocol, PurposeJa: e.ja, PurposeEn: e.en})
				}
			}
		}
	}
	// どの用途にも当たらないときは、AV でよく使う 239.x を代表として示す（推定）
	return Info{IP: str(239<<24 | 0x800000 | low), Guessed: true, Protocol: "Unknown"}
}

// withUniverse は 239.255.x.y に sACN のユニバース番号を添える（sACN ならユニバース x*256+y）。
func withUniverse(i Info) Info {
	var a, b, c, d int
	if _, err := fmt.Sscanf(i.IP, "%d.%d.%d.%d", &a, &b, &c, &d); err == nil && a == 239 && b == 255 && i.Protocol == "Dante/sACN/NDI" {
		if u := c*256 + d; u >= 1 && u <= 63999 {
			i.PurposeJa += fmt.Sprintf("。sACN ならユニバース %d", u)
			i.PurposeEn += fmt.Sprintf("; sACN universe %d if sACN", u)
		}
	}
	return i
}

func str(v uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", v>>24, v>>16&0xFF, v>>8&0xFF, v&0xFF)
}
