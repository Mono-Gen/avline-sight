package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 監査指摘#2の回帰テスト: 末尾アドレスでも周回せず終了する
func TestExpandRangeEdge(t *testing.T) {
	ips, err := expandRange("255.255.255.255")
	if err != nil || len(ips) != 1 {
		t.Fatalf("%v %v", ips, err)
	}
	if _, err := expandRange("10.0.0.0/16"); err == nil {
		t.Fatal("1024件超を拒否すべき")
	}
	ips, _ = expandRange("192.168.0.0/30")
	if len(ips) != 2 || ips[0] != "192.168.0.1" {
		t.Fatalf("/30: %v", ips)
	}
}

// 監査指摘#1の回帰テスト: PIN付きロック中は別PINで上書き→解除できない
func TestLockPINOverride(t *testing.T) {
	s := &Server{}
	call := func(body string) int {
		w := httptest.NewRecorder()
		s.lock(w, httptest.NewRequest(http.MethodPost, "/api/mode/lock", strings.NewReader(body)))
		return w.Code
	}
	if c := call(`{"locked":true,"pin":"1234"}`); c != 200 {
		t.Fatal(c)
	}
	if c := call(`{"locked":true,"pin":"9999"}`); c != 403 {
		t.Fatalf("PIN上書きが許可された: %d", c)
	}
	if c := call(`{"locked":false,"pin":"9999"}`); c != 403 {
		t.Fatalf("不正PINで解除できた: %d", c)
	}
	if c := call(`{"locked":false,"pin":"1234"}`); c != 200 || s.locked {
		t.Fatalf("正しいPINで解除できない: %d", c)
	}
}

func TestCSVSafe(t *testing.T) {
	if csvSafe("=HYPERLINK(1)") != "'=HYPERLINK(1)" || csvSafe("-3.5") != "-3.5" {
		t.Fatal("CSV数式インジェクション対策が不正")
	}
}
