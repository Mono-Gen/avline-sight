package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 上限を超える書き込みで1世代だけ残して切り替わり、2回目の切り替えで古い世代は上書きされる。
func TestRotatingFileKeepsOneGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	r, err := openRotatingFile(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"aaaaaa\n", "bbbbbb\n", "cccccc\n"} {
		if _, err := r.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	cur, _ := os.ReadFile(path)
	old, _ := os.ReadFile(path + ".1")
	if string(cur) != "cccccc\n" || string(old) != "bbbbbb\n" {
		t.Fatalf("current=%q old=%q", cur, old)
	}
}

// 起動時点で上限を超えているファイルは、書く前に切り替える。
func TestRotatingFileRotatesOversizedFileOnOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 20)), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := openRotatingFile(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	r.Write([]byte("new\n"))
	r.Close()
	cur, _ := os.ReadFile(path)
	old, _ := os.ReadFile(path + ".1")
	if string(cur) != "new\n" || len(old) != 20 {
		t.Fatalf("current=%q old=%d bytes", cur, len(old))
	}
}

// 1行が上限より大きくても、空のファイルにはそのまま書く（切り替えを繰り返さない）。
func TestRotatingFileWritesLongLineToEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	r, err := openRotatingFile(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := r.Write([]byte("longer than limit\n")); err != nil || n != 18 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	r.Close()
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("unexpected rotation: %v", err)
	}
}

// 閉じた後の書き込みはエラーにし、ファイルを開き直さない（終了処理中の書き込みで漏れないように）。
func TestRotatingFileRejectsWritesAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	r, err := openRotatingFile(path, 4)
	if err != nil {
		t.Fatal(err)
	}
	r.Write([]byte("abc\n"))
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Write([]byte("more than the limit\n")); err == nil {
		t.Fatal("write after close succeeded")
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Fatalf("rotated after close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}
