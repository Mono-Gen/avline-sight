//go:build !windows

package engine

import "context"

func preventSleep(context.Context, ...func(bool)) error { return nil }
