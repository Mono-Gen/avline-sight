package trap

import (
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestDecode(t *testing.T) {
	p := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(100)},
		{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.3"},
		{Name: ".1.3.6.1.2.1.2.2.1.1.7", Type: gosnmp.Integer, Value: 7},
	}}
	ev := Decode(p)
	if ev.Code != "linkDown" || ev.IfIndex != 7 || ev.Severity != "WARNING" {
		t.Fatalf("%+v", ev)
	}
	p.Variables[1].Value = ".1.3.6.1.4.1.9.9.9"
	ev = Decode(p)
	if ev.Code != "UNKNOWN_TRAP" || ev.Detail["trap_oid"] != "1.3.6.1.4.1.9.9.9" {
		t.Fatalf("未知Trap: %+v", ev)
	}
}
