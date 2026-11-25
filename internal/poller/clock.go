package poller

import (
	"math"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

func (w *worker) collectClock() {
	sent := time.Now()
	values, err := w.c.GetWithoutRetry([]string{
		mib.AgentSystemTime, mib.AgentSystemDate, mib.AgentSystemTimeSource,
		mib.AgentTimeZoneHoursOffset, mib.AgentTimeZoneMinutesOffset, mib.AgentSystemSummerTimeState,
		mib.AgentSummerTimeMode, mib.AgentStRecurringZoneOffset, mib.AgentStNonRecurringZoneOffset,
	})
	elapsed := time.Since(sent)
	if err != nil {
		return
	}
	// 往復遅延の中点をスイッチが値を読んだ時刻として扱う。
	if offset, ok := clockOffset(values, sent.Add(elapsed/2)); ok {
		w.st.ClockOffsetSeconds = &offset
	}
}

func clockOffset(values map[string]gosnmp.SnmpPDU, midpoint time.Time) (float64, bool) {
	// 手動設定でも時刻差は計測できるため、時刻ソースの none(0) も受け入れる。
	source, sourceOK := clockInteger(values[mib.AgentSystemTimeSource], 0, 2)
	hours, hoursOK := clockInteger(values[mib.AgentTimeZoneHoursOffset], -12, 14)
	minutes, minutesOK := clockInteger(values[mib.AgentTimeZoneMinutesOffset], 0, 59)
	summer, summerOK := clockInteger(values[mib.AgentSystemSummerTimeState], 0, 1)
	if !sourceOK || !hoursOK || !minutesOK || !summerOK {
		return 0, false
	}
	if hours < 0 {
		minutes = -minutes
	}
	zoneMinutes := hours*60 + minutes
	if summer == 1 {
		mode, ok := clockInteger(values[mib.AgentSummerTimeMode], 1, 5)
		if !ok {
			return 0, false
		}
		oid := mib.AgentStRecurringZoneOffset
		if mode == 4 {
			oid = mib.AgentStNonRecurringZoneOffset
		}
		extra, ok := clockInteger(values[oid], 0, 1440)
		if !ok {
			return 0, false
		}
		zoneMinutes += extra
	}

	date := strings.Join(strings.Fields(sx.Str(values[mib.AgentSystemDate])), " ")
	clock := strings.TrimSpace(sx.Str(values[mib.AgentSystemTime]))
	zone := time.FixedZone("switch", zoneMinutes*60)
	// 時刻を手入力(none)し、タイムゾーンも夏時間も未設定(UTC+0)のスイッチは、現地の時刻をそのまま打ち込んでいることが多い。
	// その場合はUTCと読まず、PCの表示時刻と同じ地域時刻として比べる（実機M4250: JSTの時刻を手入力・UTC+0で9時間差と誤判定した）。
	if source == 0 && zoneMinutes == 0 && summer == 0 {
		zone = midpoint.Location()
	}
	for _, layout := range []string{"01/02/2006", "Jan 2 2006", "2006-01-02"} {
		local, err := time.ParseInLocation(layout+" 15:04:05", date+" "+clock, zone)
		if err == nil {
			return math.Round(local.Sub(midpoint).Seconds()), true
		}
	}
	return 0, false
}

func clockInteger(pdu gosnmp.SnmpPDU, min, max int) (int, bool) {
	if pdu.Type != gosnmp.Integer || pdu.Value == nil {
		return 0, false
	}
	value := sx.Int(pdu)
	return int(value), value >= int64(min) && value <= int64(max)
}
