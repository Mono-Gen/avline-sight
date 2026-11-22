package engine

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/store"
)

func TestPlannedIPCSVVariants(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
		invalid          bool
	}{
		{"legacy", "MAC,名称,メモ\n00:11:22:33:44:55,卓,旧\n", "", false},
		{"ja", "MAC,名称,メモ,予定IP\n00:11:22:33:44:55,卓,,192.0.2.10\n", "192.0.2.10", false},
		{"en", "planned_ip,label,mac,note\n192.0.2.11,desk,00:11:22:33:44:55,n\n", "192.0.2.11", false},
		{"no_header", "00:11:22:33:44:55,卓,,192.0.2.12\n", "192.0.2.12", false},
		{"old_export", "MAC,名称,メモ,IP\n00:11:22:33:44:55,卓,,192.0.2.10\n", "", false},
		{"empty", "MAC,名称,メモ,予定IP\n00:11:22:33:44:55,卓,,,\n", "", false},
		{"invalid", "MAC,名称,メモ,予定IP\n00:11:22:33:44:55,卓,,192.0.2.256\n", "", true},
		{"v6", "00:11:22:33:44:55,卓,,::1\n", "", true},
		{"mapped", "00:11:22:33:44:55,卓,,::ffff:192.0.2.10\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, _, errs := ParseAliasCSV(tc.text)
			if tc.invalid {
				if len(errs) != 1 || len(rows) != 0 {
					t.Fatalf("不正値受理: %+v %+v", rows, errs)
				}
				return
			}
			if len(errs) != 0 || len(rows) != 1 || rows[0].PlannedIP != tc.want {
				t.Fatalf("rows=%+v errors=%+v", rows, errs)
			}
		})
	}
	rows := BuildInventory(nil, []store.Alias{{MatchType: "mac", MatchKey: "00:11:22:33:44:55", Label: "卓", PlannedIP: "192.0.2.10"}}, nil, InventoryOptions{Group: "*"}, time.Now())
	if len(rows) != 1 || rows[0].PlannedIP != "192.0.2.10" || rows[0].IPMismatch {
		t.Fatalf("未観測予定IP: %+v", rows)
	}
	var b bytes.Buffer
	if err := WriteInventoryCSV(&b, rows, time.UTC); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "MAC,名称,メモ,予定IP,IP") {
		t.Fatal("4列目なし")
	}
	parsed, _, errs := ParseAliasCSV(b.String())
	if len(errs) != 0 || len(parsed) != 1 || parsed[0].PlannedIP != "192.0.2.10" {
		t.Fatalf("往復: %+v %+v", parsed, errs)
	}
}
