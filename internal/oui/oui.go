// Package oui はMAC先頭3オクテットから主要AV・ネットワークベンダー名をオフライン解決する。
// 収録は主要ベンダーの代表的なOUIのみ（IEEE登録簿の部分集合）。
// TODO(要確認): QSC・Biamp・MA Lighting・Clear-Com・BirdDog等は確度の高いOUIを確認できていないため未収録。実運用前に IEEE OUI 登録簿（standards-oui.ieee.org）と突合し、各社の追加OUIを補充すること。
package oui

import "strings"

var table = map[string]string{
	"001DC1": "Audinate",
	"00A0DE": "Yamaha",
	"000EDD": "Shure",
	"7C2E0D": "Blackmagic Design",
	"008045": "Panasonic",
	"080046": "Sony",
	"00107F": "Crestron",
	"0005A6": "Extron",
	"00095B": "Netgear",
	"28C68E": "Netgear",
	"A040A0": "Netgear",
	"9C3DCF": "Netgear",
	"E091F5": "Netgear",
	"000C29": "VMware",
	"001B21": "Intel",
	"00E04C": "Realtek",
}

// Lookup はMAC（区切り任意）からベンダー名を返す。未収録・不正値は空文字。
func Lookup(mac string) string {
	h := strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "").Replace(mac))
	if len(h) < 6 {
		return ""
	}
	return table[h[:6]]
}
