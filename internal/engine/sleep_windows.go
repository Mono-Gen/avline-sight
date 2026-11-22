//go:build windows

package engine

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
)

func preventSleep(ctx context.Context, notify ...func(bool)) error {
	if ctx.Err() != nil {
		return nil
	}
	// Execution stateは呼出スレッドに属するため、取得と解除を同じOSスレッドで行う。
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	setState := syscall.NewLazyDLL("kernel32.dll").NewProc("SetThreadExecutionState")
	const continuous = 0x80000000
	const systemRequired = 0x00000001
	if result, _, err := setState.Call(continuous | systemRequired); result == 0 {
		return fmt.Errorf("prevent system sleep: %w", err)
	}
	for _, setActive := range notify {
		setActive(true)
		defer setActive(false)
	}
	<-ctx.Done()
	if result, _, err := setState.Call(continuous); result == 0 {
		return fmt.Errorf("release sleep prevention: %w", err)
	}
	return nil
}
