//go:build !windows

package engine

import "context"

func preventSleep(context.Context) error { return nil }
