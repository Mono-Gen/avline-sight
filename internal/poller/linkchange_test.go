package poller

import (
	"testing"
	"time"
)

// 期待値は定義から: 変化時刻 = 今 − (今のsysUpTime − ifLastChange)。今のsysUpTime = 取得時の値 + 取得からの経過。
func TestLinkChangeTime(t *testing.T) {
	now := time.Date(2026, 11, 25, 17, 0, 0, 0, time.UTC)
	uptimeAt := now.Add(-30 * time.Second) // 30秒前に sysUpTime = 1時間 を取得
	const hour = int64(3600 * 100)
	// 起動から10分後に変化 → 今の稼働は1時間30秒なので、50分30秒前
	got, ok := linkChangeTime(now, uptimeAt, hour, uint64(600*100))
	if want := now.Add(-(50*time.Minute + 30*time.Second)); !ok || !got.Equal(want) {
		t.Fatalf("got %v %v, want %v", got, ok, want)
	}
	// 起動直後から変化なし（0）→ 起動時刻
	if got, ok := linkChangeTime(now, uptimeAt, hour, 0); !ok || !got.Equal(now.Add(-(time.Hour + 30*time.Second))) {
		t.Fatalf("since boot: %v %v", got, ok)
	}
	// sysUpTime 未取得 / 変化が未来（再起動直後の古い値）→ 求めない
	if _, ok := linkChangeTime(now, time.Time{}, hour, 0); ok {
		t.Fatal("computed without an uptime sample")
	}
	if _, ok := linkChangeTime(now, uptimeAt, hour, uint64(2*hour)); ok {
		t.Fatal("computed a change in the future")
	}
}
