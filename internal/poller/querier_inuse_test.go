package poller

import (
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

// 実機（TechSupport）: VLAN・全体とも 0.0.0.0 の querier は管理IPで動き、non-querier は選出された相手を見る。
func TestQuerierInUse(t *testing.T) {
	cases := []struct {
		name               string
		v                  model.VlanIGMP
		global, mgmt, want string
		guessed            bool
	}{
		{"VLANのアドレスが優先", model.VlanIGMP{QuerierOper: "querier", QuerierAddr: "10.0.11.1"}, "10.0.0.9", "10.0.0.1", "10.0.11.1", false},
		{"全体のアドレス", model.VlanIGMP{QuerierOper: "querier", QuerierAddr: "0.0.0.0"}, "10.0.0.9", "10.0.0.1", "10.0.0.9", false},
		{"どちらも未設定なら管理IP（推定）", model.VlanIGMP{QuerierOper: "querier", QuerierAddr: "0.0.0.0", LastQuerier: "192.168.254.254"}, "", "192.168.254.1", "192.168.254.1", true},
		{"non-querierは選出された相手", model.VlanIGMP{QuerierOper: "non-querier", LastQuerier: "192.168.254.1"}, "", "192.168.254.254", "192.168.254.1", false},
		{"non-querierで相手不明", model.VlanIGMP{QuerierOper: "non-querier", LastQuerier: "0.0.0.0"}, "", "192.168.254.254", "", false},
		{"無効", model.VlanIGMP{QuerierOper: "disabled", LastQuerier: "192.168.254.254"}, "", "192.168.254.1", "", false},
	}
	for _, c := range cases {
		ip, g := querierInUse(&c.v, c.global, c.mgmt)
		if ip != c.want || g != c.guessed {
			t.Errorf("%s: got %q,%v want %q,%v", c.name, ip, g, c.want, c.guessed)
		}
	}
}
