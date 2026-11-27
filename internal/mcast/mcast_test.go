package mcast

import "testing"

// 期待値は実機（2026-11-27）で観測した MAC と、IANA・Audinate・OpenAvnu 等の資料による。
func TestLookupObservedMACs(t *testing.T) {
	cases := []struct{ in, ip, proto string }{
		{"01:00:5E:00:00:12", "224.0.0.18", "VRRP"},
		{"01:00:5E:00:00:E6", "224.0.0.230", "Dante"},
		{"01:00:5E:00:00:E9", "224.0.0.233", "Dante"},
		{"01:00:5E:00:00:FB", "224.0.0.251", "mDNS"},
		{"01:00:5E:00:00:FC", "224.0.0.252", "LLMNR"},
		{"01:00:5E:00:01:81", "224.0.1.129", "PTP"},
		{"01:00:5E:7F:FF:FA", "239.255.255.250", "SSDP"},
		{"01:00:5E:7F:FF:FF", "239.255.255.255", "SAP"},
		{"01:00:5E:45:01:02", "239.69.1.2", "AES67"},
		{"91:E0:F0:00:FF:00", "", "AVB MAAP"},
		{"91:E0:F0:01:00:00", "", "AVDECC"},
	}
	for _, c := range cases {
		got := Lookup(c.in)
		if got.IP != c.ip || got.Protocol != c.proto || got.PurposeJa == "" && c.proto != "Unknown" {
			t.Errorf("%s: got %+v, want ip=%s proto=%s", c.in, got, c.ip, c.proto)
		}
		if c.ip != "" && !got.Guessed {
			t.Errorf("%s: MAC から引いた IP は推定扱いのはず", c.in)
		}
	}
}

func TestLookupIPAndUniverse(t *testing.T) {
	got := Lookup("239.255.0.1")
	if got.Guessed || got.Protocol != "Dante/sACN/NDI" || got.PurposeJa == "" {
		t.Fatalf("got %+v", got)
	}
	if want := "ユニバース 1"; !contains(got.PurposeJa, want) {
		t.Fatalf("sACN ユニバースが付かない: %s", got.PurposeJa)
	}
	if u := Lookup("not-a-group"); u.Protocol != "Unknown" {
		t.Fatalf("got %+v", u)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
