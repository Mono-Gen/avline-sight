//go:build windows

package ping

import (
	"encoding/binary"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"
)

// WindowsのICMP API（iphlpapi.dll）を直接呼ぶ。管理者権限が不要で、ping.exe の出力（言語依存）を
// 解析せずに済み、「宛先ホストに到達できません」の応答を成功と取り違えない。
var (
	iphlpapi        = syscall.NewLazyDLL("iphlpapi.dll")
	procIcmpCreate  = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpSend    = iphlpapi.NewProc("IcmpSendEcho")
	procIcmpClose   = iphlpapi.NewProc("IcmpCloseHandle")
	invalidHandle   = ^uintptr(0)
	echoPayload     = []byte("AVLine-Sight ping")
	replyBufferSize = 256 // ICMP_ECHO_REPLY（64bitで40バイト）＋ペイロード＋8 を十分に上回る
)

// IP_STATUS の値（ipexport.h）
const (
	ipSuccess             = 0
	ipDestNetUnreachable  = 11002
	ipDestHostUnreachable = 11003
	ipDestProtUnreachable = 11004
	ipDestPortUnreachable = 11005
	ipReqTimedOut         = 11010
	ipTTLExpiredTransit   = 11013
)

func echo(v4 net.IP, timeout time.Duration) Result {
	h, _, err := procIcmpCreate.Call()
	if h == invalidHandle || h == 0 {
		return Result{Status: StatusError, RTTms: -1, Detail: fmt.Sprintf("ICMPハンドルを開けません: %v", err)}
	}
	defer procIcmpClose.Call(h)
	// IPAddr はネットワークバイト順の in_addr。メモリ上の並びをそのまま uint32 にする
	dst := binary.LittleEndian.Uint32(v4)
	buf := make([]byte, replyBufferSize)
	n, _, callErr := procIcmpSend.Call(h, uintptr(dst),
		uintptr(unsafe.Pointer(&echoPayload[0])), uintptr(len(echoPayload)), 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(timeout.Milliseconds()))
	if n == 0 {
		// 応答0件: GetLastError に IP_STATUS が入る
		if errno, ok := callErr.(syscall.Errno); ok {
			return classify(uint32(errno), -1)
		}
		return Result{Status: StatusError, RTTms: -1, Detail: fmt.Sprint(callErr)}
	}
	// ICMP_ECHO_REPLY の先頭: Address(4) Status(4) RoundTripTime(4)
	from := net.IP(buf[0:4]).String()
	status := binary.LittleEndian.Uint32(buf[4:8])
	rtt := float64(binary.LittleEndian.Uint32(buf[8:12]))
	r := classify(status, rtt)
	if r.Status == StatusOK && !net.IP(buf[0:4]).Equal(v4) {
		// 宛先以外からの応答は成功扱いにしない（念のため）
		return Result{Status: StatusUnreachable, RTTms: -1, Detail: "宛先以外（" + from + "）から応答"}
	}
	if r.Status == StatusUnreachable {
		r.Detail += "（通知元 " + from + "）"
	}
	return r
}

func classify(status uint32, rtt float64) Result {
	switch status {
	case ipSuccess:
		return Result{Status: StatusOK, RTTms: rtt}
	case ipReqTimedOut:
		return Result{Status: StatusTimeout, RTTms: -1}
	case ipDestNetUnreachable, ipDestHostUnreachable, ipDestProtUnreachable, ipDestPortUnreachable, ipTTLExpiredTransit:
		return Result{Status: StatusUnreachable, RTTms: -1, Detail: fmt.Sprintf("到達不能（IP_STATUS %d）", status)}
	}
	return Result{Status: StatusError, RTTms: -1, Detail: fmt.Sprintf("IP_STATUS %d", status)}
}
