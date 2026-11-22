package trap

import (
	"net"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/gosnmp/gosnmp"
)

func TestDecode(t *testing.T) {
	p := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(100)},
		{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.3"},
		{Name: ".1.3.6.1.2.1.2.2.1.1.7", Type: gosnmp.Integer, Value: 7},
	}}
	ev := Decode(p)
	if ev.Code != "linkDown" || ev.IfIndex != 7 || ev.Severity != "WARNING" || ev.TrapUptime == nil || *ev.TrapUptime != 100 {
		t.Fatalf("%+v", ev)
	}
	p.Variables[1].Value = ".1.3.6.1.4.1.9.9.9"
	ev = Decode(p)
	if ev.Code != "UNKNOWN_TRAP" || ev.Detail["trap_oid"] != "1.3.6.1.4.1.9.9.9" {
		t.Fatalf("未知Trap: %+v", ev)
	}
}

func TestTrapTriggerOnlyForRegisteredSource(t *testing.T) {
	var events, triggers []model.Event
	r := New(func(ev model.Event) { events = append(events, ev) }, func(ip string, _ int) (int64, string, bool) { return 7, "SW", ip == "192.0.2.1" })
	r.OnRegistered(func(ev model.Event) { triggers = append(triggers, ev) })
	p := &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{{Name: ".1.3.6.1.6.3.1.1.4.1.0", Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.6.3.1.1.5.5"}}}
	r.handle(p, &net.UDPAddr{IP: net.ParseIP("192.0.2.2")})
	if len(events) != 1 || len(triggers) != 0 || events[0].Detail["unregistered_source"] != "192.0.2.2" {
		t.Fatalf("未登録: events=%+v triggers=%+v", events, triggers)
	}
	r.handle(p, &net.UDPAddr{IP: net.ParseIP("192.0.2.1")})
	if len(events) != 2 || len(triggers) != 1 || triggers[0].SwitchID != 7 || events[1].Code != "authenticationFailure" || events[1].Severity != "INFO" {
		t.Fatalf("登録済み: events=%+v triggers=%+v", events, triggers)
	}
}
