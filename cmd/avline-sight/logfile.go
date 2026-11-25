package main

import (
	"fmt"
	"os"
	"sync"
)

// logFileLimit はログファイルの上限。超えたら1世代だけ残して切り替える。
// 10MBは、監視ログ（1行200バイト程度）で数万行分あり、障害の前後を追うには足りる量。
const logFileLimit = 10 << 20

// rotatingFile はサイズ上限で1世代だけ残して切り替えるログファイル。
// 黒い画面を出さない起動ではログの行き先がここだけになる。
type rotatingFile struct {
	mu    sync.Mutex
	path  string
	limit int64
	f     *os.File
	size  int64
	// 切り替えに失敗したら（古い世代を別のプロセスが開いている等）以後は切り替えず書き続ける。
	// 書くたびに閉じる・開くを繰り返さないため。
	noRotate bool
	closed   bool
}

func openRotatingFile(path string, limit int64) (*rotatingFile, error) {
	r := &rotatingFile{path: path, limit: limit}
	if err := r.open(); err != nil {
		return nil, err
	}
	// 起動時点で上限を超えていれば先に切り替える。失敗してもログ自体は止めない
	if r.size >= limit {
		if err := r.rotate(); err != nil {
			r.noRotate = true
			if r.f == nil {
				if err := r.open(); err != nil {
					return nil, err
				}
			}
		}
	}
	return r, nil
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("ログファイルを開けません %s: %w", r.path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("ログファイルの大きさを読めません %s: %w", r.path, err)
	}
	r.f, r.size = f, info.Size()
	return nil
}

// rotate は今のファイルを .1 にして新しいファイルを開く（古い .1 は上書き）。
func (r *rotatingFile) rotate() error {
	err := r.f.Close()
	r.f = nil
	if err != nil {
		return fmt.Errorf("ログファイルを閉じられません: %w", err)
	}
	old := r.path + ".1"
	if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("古いログを消せません %s: %w", old, err)
	}
	if err := os.Rename(r.path, old); err != nil {
		return fmt.Errorf("ログを切り替えられません: %w", err)
	}
	return r.open()
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, os.ErrClosed
	}
	if !r.noRotate && r.size > 0 && r.size+int64(len(p)) > r.limit {
		if err := r.rotate(); err != nil {
			// 切り替えに失敗しても監視は止めない。以後は切り替えず、今のファイルへ書き続ける
			r.noRotate = true
			if r.f == nil {
				if openErr := r.open(); openErr != nil {
					return 0, err
				}
			}
		}
	}
	// 前回の開き直しが一時的に失敗していたら、ここでもう一度試す
	if r.f == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.f == nil {
		return nil
	}
	return r.f.Close()
}
