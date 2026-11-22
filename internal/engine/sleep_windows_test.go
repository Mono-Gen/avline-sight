//go:build windows

package engine

import (
	"context"
	"testing"
	"time"
)

func TestWindowsSleepPreventionReleasesOnCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := preventSleep(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("終了前に抑止スレッドが終了した")
	}
}
