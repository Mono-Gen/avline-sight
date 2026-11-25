package poller

import (
	"testing"

	"github.com/gosnmp/gosnmp"
)

func octets(b []byte) gosnmp.SnmpPDU { return gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: b} }

// PCのLLDPは chassis ID にホスト名(local)、port ID にMACのバイト列を送る(実機 M4250 で観測)。
func TestLLDPIDDecodesBySubtype(t *testing.T) {
	mac := []byte{0x02, 0x00, 0x5E, 0x10, 0x20, 0x30}
	if got, isMAC := lldpID(octets(mac), lldpPortMAC, lldpPortMAC, false); got != "02:00:5E:10:20:30" || !isMAC {
		t.Fatalf("port MAC subtype: %q %v", got, isMAC)
	}
	// ホスト名がちょうど6文字でも、subtype が local なら文字列のまま。
	if got, isMAC := lldpID(octets([]byte("SW-001")), 7, lldpChassisMAC, true); got != "SW-001" || isMAC {
		t.Fatalf("local chassis: %q %v", got, isMAC)
	}
	// subtype が読めない機器: chassis は従来どおり6バイトをMACとみなす。
	if got, _ := lldpID(octets(mac), 0, lldpChassisMAC, true); got != "02:00:5E:10:20:30" {
		t.Fatalf("chassis without subtype: %q", got)
	}
	// subtype が読めない port ID: 6文字のポート名はMACにしない、バイナリはMACにする。
	if got, isMAC := lldpID(octets([]byte("port-1")), 0, lldpPortMAC, false); got != "port-1" || isMAC {
		t.Fatalf("printable port without subtype: %q %v", got, isMAC)
	}
	if got, isMAC := lldpID(octets(mac), 0, lldpPortMAC, false); got != "02:00:5E:10:20:30" || !isMAC {
		t.Fatalf("binary port without subtype: %q %v", got, isMAC)
	}
}

func TestLLDPDeviceKeyMergesWithFDBByMAC(t *testing.T) {
	if got := lldpDeviceKey("PC-HOST", "02:00:5E:10:20:30", true); got != "02:00:5E:10:20:30" {
		t.Fatalf("hostname chassis + MAC port: %q", got)
	}
	if got := lldpDeviceKey("02:00:5E:AA:BB:CC", "02:00:5E:10:20:30", true); got != "02:00:5E:AA:BB:CC" {
		t.Fatalf("MAC chassis wins: %q", got)
	}
	if got := lldpDeviceKey("PC-HOST", "Ethernet 2", false); got != "PC-HOST" {
		t.Fatalf("no MAC anywhere: %q", got)
	}
}
