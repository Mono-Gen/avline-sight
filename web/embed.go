// Package web はビルド済みWebUI（web/dist）を実行ファイルへ同梱する。
// 事前に `npm run build`（web/）で dist を生成しておくこと（scripts/build.ps1 が自動実行）。
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Dist は dist 直下をルートとするファイルシステム。
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
