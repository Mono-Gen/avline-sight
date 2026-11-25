package poller

import (
	"math"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/sim"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

func TestClockOffsetCorrectsTimezoneAndDaylightSaving(t *testing.T) {
	integer := func(n int) gosnmp.SnmpPDU { return gosnmp.SnmpPDU{Type: gosnmp.Integer, Value: n} }
	str := func(s string) gosnmp.SnmpPDU { return gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: []byte(s)} }
	values := map[string]gosnmp.SnmpPDU{
		mib.AgentSystemTime: str("23:01:05"), mib.AgentSystemDate: str("Jul 1 2025"),
		mib.AgentSystemTimeSource: integer(0), mib.AgentSystemSummerTimeState: integer(1),
		mib.AgentTimeZoneHoursOffset: integer(-3), mib.AgentTimeZoneMinutesOffset: integer(30),
		mib.AgentSummerTimeMode: integer(4), mib.AgentStNonRecurringZoneOffset: integer(60),
	}
	midpoint := time.Date(2025, 7, 2, 1, 30, 0, 0, time.UTC)
	if offset, ok := clockOffset(values, midpoint); !ok || offset != 65 {
		t.Fatalf("offset %v, valid %v; want 65 seconds", offset, ok)
	}
	values[mib.AgentSummerTimeMode] = integer(3)
	values[mib.AgentStRecurringZoneOffset] = integer(60)
	if offset, ok := clockOffset(values, midpoint); !ok || offset != 65 {
		t.Fatalf("recurring offset %v, valid %v; want 65 seconds", offset, ok)
	}
	delete(values, mib.AgentStRecurringZoneOffset)
	if _, ok := clockOffset(values, midpoint); ok {
		t.Fatal("accepted unknown active summertime offset")
	}
}

func TestClockCollectionLabFaultAndFailedGET(t *testing.T) {
	ctx := t.Context()
	const port = 27182
	lab := sim.NewLab(1, 1)
	if err := lab.Start(ctx, sim.Config{BaseSNMPPort: port}); err != nil {
		t.Fatal(err)
	}
	c, err := sx.DialContext(ctx, "127.0.0.1", port, "public", time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c != nil {
			if err := c.Close(); err != nil {
				t.Error(err)
			}
		}
	})
	w := worker{c: c, st: &model.SwitchState{}}
	check := func(want float64) {
		t.Helper()
		w.collectClock()
		if offset := w.st.ClockOffsetSeconds; offset == nil || math.Abs(*offset-want) > 1 {
			t.Fatalf("clock offset = %v, want about %v", offset, want)
		}
	}
	check(0)
	if err := lab.Inject(0, "clock-skew"); err != nil {
		t.Fatal(err)
	}
	check(120)
	lab.Reset()
	check(0)
	previous := w.st.ClockOffsetSeconds
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	c = nil
	w.collectClock()
	if w.st.ClockOffsetSeconds != previous {
		t.Fatal("failed GET replaced previous clock sample")
	}
	w.st.ClockOffsetSeconds = nil
	w.collectClock()
	if w.st.ClockOffsetSeconds != nil {
		t.Fatal("failed initial GET produced a clock sample")
	}
}

// 手入力・タイムゾーン未設定のスイッチは、表示時刻をPCの地域時刻として比べる。SNTPやタイムゾーン設定済みなら従来どおり。
func TestClockOffsetManualClockWithoutTimezoneUsesPCLocalTime(t *testing.T) {
	integer := func(n int) gosnmp.SnmpPDU { return gosnmp.SnmpPDU{Type: gosnmp.Integer, Value: n} }
	str := func(s string) gosnmp.SnmpPDU { return gosnmp.SnmpPDU{Type: gosnmp.OctetString, Value: []byte(s)} }
	jst := time.FixedZone("JST", 9*3600)
	values := map[string]gosnmp.SnmpPDU{
		mib.AgentSystemTime: str("18:50:49"), mib.AgentSystemDate: str(" Nov 25 2026"),
		mib.AgentSystemTimeSource: integer(0), mib.AgentSystemSummerTimeState: integer(0),
		mib.AgentTimeZoneHoursOffset: integer(0), mib.AgentTimeZoneMinutesOffset: integer(0),
	}
	midpoint := time.Date(2026, 11, 25, 18, 50, 29, 0, jst)
	if offset, ok := clockOffset(values, midpoint); !ok || offset != 20 {
		t.Fatalf("manual clock without timezone: offset %v, valid %v; want 20 seconds", offset, ok)
	}
	// SNTPで合わせている(1)ならスイッチの時刻はUTCそのもの: 9時間進んでいると判定する。
	values[mib.AgentSystemTimeSource] = integer(1)
	if offset, ok := clockOffset(values, midpoint); !ok || offset != 9*3600+20 {
		t.Fatalf("SNTP clock: offset %v, valid %v; want 9h20s", offset, ok)
	}
}
