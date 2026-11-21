package snmpx

import "testing"

func TestCounterDelta(t *testing.T) {
	cases := []struct {
		prev, cur uint64
		is32      bool
		want      uint64
		ok        bool
	}{
		{100, 150, false, 50, true},
		{0xfffffff0, 0x10, true, 0x20, true}, // 32bitラップ
		{1000, 10, false, 0, false},           // 64bitの減少はリセット扱い
		{1 << 31, 5, true, 0, false},          // 32bitでも差が巨大ならリセット扱い
	}
	for _, c := range cases {
		got, ok := CounterDelta(c.prev, c.cur, c.is32)
		if got != c.want || ok != c.ok {
			t.Errorf("Delta(%d,%d,%v)=%d,%v want %d,%v", c.prev, c.cur, c.is32, got, ok, c.want, c.ok)
		}
	}
}
