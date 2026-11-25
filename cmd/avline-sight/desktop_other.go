//go:build !windows

package main

import (
	"fmt"
	"os"
)

// Windows 以外はこれまでどおり端末で動かす（トレイ・メッセージボックス・二重起動防止なし）。

func attachConsole() bool { return true }

func showStartupMessage(text string, _ bool) { fmt.Fprintln(os.Stderr, text) }

func acquireSingleInstance(string) (func(), error) { return func() {}, nil }

func startTray(string, func()) (func(), error) { return func() {}, nil }
