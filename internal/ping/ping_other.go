//go:build !windows

package ping

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// Windows以外は raw ICMP に特権が要るため、OS の ping コマンドで代替する。
// NOTE: 主対象はWindows。こちらは終了コード（0=応答あり）と出力の time= を見る簡易実装。
var rttRe = regexp.MustCompile(`time[=<]\s*([\d.]+)\s*ms`)

func echo(v4 net.IP, timeout time.Duration) Result {
	sec := int((timeout + time.Second - 1) / time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), timeout+2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ping", "-c", "1", "-W", strconv.Itoa(sec), v4.String()).CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return Result{Status: StatusTimeout, RTTms: -1}
		}
		return Result{Status: StatusError, RTTms: -1, Detail: fmt.Sprint(err)}
	}
	rtt := -1.0
	if m := rttRe.FindSubmatch(out); m != nil {
		rtt, _ = strconv.ParseFloat(string(m[1]), 64)
	}
	return Result{Status: StatusOK, RTTms: rtt}
}
