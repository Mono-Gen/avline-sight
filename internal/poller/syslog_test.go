package poller

import (
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// 期待値は RFC 5424 の重要度（PRI mod 8）と syslogSeverity の対応表から導く。
func TestSyslogSeverity(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"local7 emerg", "<184>Oct 30 07:26:02 x: boom", model.SevCritical},
		{"local7 crit", "<186>Oct 30 07:26:02 x: crit", model.SevCritical},
		{"local7 err", "<187>Oct 30 07:26:02 x: err", model.SevWarning},
		// 実機 M4250 のログ: "%% WARN Buffer pool is running out"
		{"local7 warning (real M4250)", "<188>Oct 30 07:26:04 0.0.0.0-1 DRIVER[bcmRX]: rx_pool.c(533) 37 %% WARN Buffer pool is running out: total 640, free 8", model.SevWarning},
		{"local7 notice (real M4250)", "<189>Oct 30 07:26:02 0.0.0.0-1 General[fp_main_task]: sdm_template_mgr.c(574) 5 %% NOTE Booting with default SDM template", model.SevInfo},
		{"local7 debug", "<191>x", model.SevInfo},
		{"kern emerg (PRI 0)", "<0>x", model.SevCritical},
		{"no PRI", "plain message", model.SevInfo},
		{"empty", "", model.SevInfo},
		{"empty PRI", "<>x", model.SevInfo},
		{"not a number", "<ab>x", model.SevInfo},
		{"out of range", "<192>x", model.SevInfo},
		{"too many digits", "<0188>x", model.SevInfo},
		{"no closing bracket", "<188 x", model.SevInfo},
		{"negative", "<-1>x", model.SevInfo},
	}
	for _, c := range cases {
		if got := syslogSeverity(c.text); got != c.want {
			t.Errorf("%s: syslogSeverity(%q) = %s, want %s", c.name, c.text, got, c.want)
		}
	}
}
