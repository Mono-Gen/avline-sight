package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/engine"
	"github.com/Mono-Gen/avline-sight/internal/model"
	"github.com/Mono-Gen/avline-sight/internal/store"
)

func newDevicesServer(t *testing.T) (*Server, http.Handler) {
	st, err := store.Open(filepath.Join(t.TempDir(), "d.db"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	pid, _ := st.EnsureDefaultProfile()
	sw := model.SwitchConfig{ProfileID: pid, Name: "SW1", IP: "10.0.0.1", Enabled: true}
	st.SaveSwitch(&sw)
	e := engine.New(st, pid, nil)
	e.Reload()
	e.OnState(&model.SwitchState{ID: sw.ID, Name: "SW1", Reachable: true, Ports: []*model.Port{
		{IfIndex: 1, Name: "1/0/1", Oper: "up", Devices: []model.Device{{MAC: "00:11:22:33:44:55", Source: "fdb"}}}}}, "slow")
	s := &Server{Store: st, Engine: e, replays: map[string]*replay{}}
	return s, s.Handler()
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// 試行→確定の流れ、エラー行があると確定を拒否、ロック中は拒否、書出に登録名が反映される
func TestAliasImportFlow(t *testing.T) {
	s, h := newDevicesServer(t)
	csv := func(c string) string { b, _ := json.Marshal(c); return string(b) }

	w := do(h, "POST", "/api/device-aliases/import", `{"csv":`+csv("MAC,名称\n00-11-22-33-44-55,卓\nxx,不正\n")+`,"commit":true}`)
	if w.Code != 422 {
		t.Fatalf("エラー行ありの確定は拒否すべき: %d %s", w.Code, w.Body)
	}
	if al, _ := s.Store.Aliases(s.Engine.ProfileID()); len(al) != 0 {
		t.Fatal("拒否したのに書き込まれた")
	}
	body := `{"csv":` + csv("MAC,名称\n00-11-22-33-44-55,卓\n") + `}`
	w = do(h, "POST", "/api/device-aliases/import", body)
	var pv struct {
		Committed bool
		Rows      []struct{ MAC, Action string }
	}
	json.Unmarshal(w.Body.Bytes(), &pv)
	if w.Code != 200 || pv.Committed || len(pv.Rows) != 1 || pv.Rows[0].Action != "add" {
		t.Fatalf("試行: %d %s", w.Code, w.Body)
	}
	if al, _ := s.Store.Aliases(s.Engine.ProfileID()); len(al) != 0 {
		t.Fatal("試行で書き込まれた")
	}

	s.locked = true
	if w = do(h, "POST", "/api/device-aliases/import", strings.Replace(body, "}", `,"commit":true}`, 1)); w.Code != http.StatusLocked {
		t.Fatalf("ロック中の確定: %d", w.Code)
	}
	s.locked = false
	if w = do(h, "POST", "/api/device-aliases/import", strings.Replace(body, "}", `,"commit":true}`, 1)); w.Code != 200 {
		t.Fatalf("確定: %d %s", w.Code, w.Body)
	}

	w = do(h, "GET", "/api/devices/export.csv?scope=all", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "00:11:22:33:44:55,卓,") || !strings.Contains(w.Header().Get("Content-Disposition"), "avline-sight-devices-") {
		t.Fatalf("書出: %d %q %s", w.Code, w.Header().Get("Content-Disposition"), w.Body)
	}
	w = do(h, "GET", "/api/devices?scope=unnamed", "")
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("登録後の名前なし一覧は空のはず: %s", w.Body)
	}
}

// 一括Ping: 未知のIPは拒否、短縮範囲と上限、ロック中はすべて拒否
func TestPingBulk(t *testing.T) {
	s, h := newDevicesServer(t)
	// スイッチIP（10.0.0.1）だけが既知。ループバックは手入力でのみ打てる
	if w := do(h, "POST", "/api/diagnostics/ping-bulk", `{"ips":["127.0.0.1"]}`); w.Code != 403 {
		t.Fatalf("未知IPを一覧扱いで受理: %d", w.Code)
	}
	w := do(h, "POST", "/api/diagnostics/ping-bulk", `{"manual":"127.0.0.1"}`)
	var res struct {
		OK, Total int
		Results   []struct{ IP, Status, Source string }
	}
	json.Unmarshal(w.Body.Bytes(), &res)
	if w.Code != 200 || res.Total != 1 || res.OK != 1 || res.Results[0].Source != "manual" {
		t.Fatalf("手入力: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/api/diagnostics/ping-bulk", `{"manual":"10.1.0.0/23"}`); w.Code != 400 {
		t.Fatalf("上限超過（510件）を受理: %d", w.Code)
	}
	if w := do(h, "POST", "/api/diagnostics/ping-bulk", `{"manual":"bad"}`); w.Code != 400 {
		t.Fatalf("不正な手入力: %d", w.Code)
	}
	s.locked = true
	if w := do(h, "POST", "/api/diagnostics/ping-bulk", `{"manual":"127.0.0.1"}`); w.Code != http.StatusLocked {
		t.Fatalf("ロック中の手入力: %d", w.Code)
	}
	if w := do(h, "POST", "/api/diagnostics/ping-bulk", `{"ips":["10.0.0.1"]}`); w.Code != http.StatusLocked {
		t.Fatalf("ロック中は既知IPへのPingも禁止（ユーザー判断 2026-11-22）: %d", w.Code)
	}
	if w := do(h, "POST", "/api/diagnostics/ping", `{"ip":"10.0.0.1"}`); w.Code != http.StatusLocked {
		t.Fatalf("ロック中は1台版Pingも禁止: %d", w.Code)
	}
}

func TestExpandRangeShortForm(t *testing.T) {
	ips, err := expandRange("192.168.1.10-12")
	if err != nil || len(ips) != 3 || ips[2] != "192.168.1.12" {
		t.Fatalf("%v %v", ips, err)
	}
	if _, err := expandRange("192.168.1.10-5"); err == nil {
		t.Fatal("逆順を受理")
	}
}
