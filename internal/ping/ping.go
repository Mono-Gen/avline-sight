// Package ping はICMPエコーによる疎通確認。
// 責任: 1台へのエコー送受信と、上限つき並列の一括実行。
// 対象外: 宛先の許可判定（api側で監視中の機器・登録機器・手入力の扱いを決める）、結果の保存。
package ping

import (
	"context"
	"net"
	"sync"
	"time"
)

// Status はPing結果の分類。「無応答」は故障を意味しない（経路がない、ICMPを遮断している等もある）。
type Status string

const (
	StatusOK          Status = "ok"          // 宛先から応答あり
	StatusTimeout     Status = "timeout"     // 時間内に応答なし
	StatusUnreachable Status = "unreachable" // 途中の機器から到達不能の通知
	StatusError       Status = "error"       // 送信自体ができない（引数不正・OSエラー）
)

// Result は1宛先の結果。
type Result struct {
	IP     string  `json:"ip"`
	Status Status  `json:"status"`
	RTTms  float64 `json:"rtt_ms"` // 応答なしは -1
	Detail string  `json:"detail,omitempty"`
}

// DefaultTimeout は1回のエコー待ち時間。AV機器のLANでは通常数msで返るため、1秒で打ち切る。
const DefaultTimeout = time.Second

// Attempts は1宛先あたりの最大送信回数。1回目の取りこぼし（ARP解決待ち等）で無応答としないため2回。
const Attempts = 2

// MaxParallel は同時に打つ宛先数。現場ネットワークに瞬間的な負荷をかけないための上限。
const MaxParallel = 16

// One は ip へ最大 Attempts 回エコーを送り、最初の応答で返す。
func One(ctx context.Context, ip string, timeout time.Duration) Result {
	v4 := net.ParseIP(ip).To4()
	if v4 == nil {
		return Result{IP: ip, Status: StatusError, RTTms: -1, Detail: "IPv4アドレスではありません"}
	}
	var last Result
	for i := 0; i < Attempts; i++ {
		if ctx.Err() != nil {
			return Result{IP: ip, Status: StatusError, RTTms: -1, Detail: "中断されました"}
		}
		last = echo(v4, timeout)
		last.IP = ip
		if last.Status == StatusOK || last.Status == StatusError {
			return last
		}
	}
	return last
}

// Many は ips を MaxParallel 並列で打ち、入力と同じ順で結果を返す。
func Many(ctx context.Context, ips []string, timeout time.Duration) []Result {
	out := make([]Result, len(ips))
	sem := make(chan struct{}, MaxParallel)
	var wg sync.WaitGroup
	for i, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = One(ctx, ip, timeout)
		}()
	}
	wg.Wait()
	return out
}
