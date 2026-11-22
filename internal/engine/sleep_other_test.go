//go:build !windows

package engine

import (
	"context"
	"testing"
)

func TestSleepPreventionNoop(t *testing.T) {
	if err := preventSleep(context.Background()); err != nil {
		t.Fatal(err)
	}
}
