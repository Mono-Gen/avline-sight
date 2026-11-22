package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Mono-Gen/avline-sight/internal/model"
)

func TestSwitchSaveValidatesGroupAndNotifiesLiveClients(t *testing.T) {
	s, h := newDevicesServer(t)
	s.Hub = NewHub()
	c := &client{send: make(chan []byte, 8)}
	s.Hub.clients[c] = true
	switchBody := func(group string) string {
		t.Helper()
		b, err := json.Marshal(model.SwitchConfig{Name: "switch", IP: "192.0.2.2", GroupTag: group, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	checkNotice := func() {
		t.Helper()
		select {
		case b := <-c.send:
			var msg struct{ Type string }
			if err := json.Unmarshal(b, &msg); err != nil || msg.Type != "groups_changed" {
				t.Fatalf("グループ変更通知: %s (%v)", b, err)
			}
		default:
			t.Fatal("グループ変更通知なし")
		}
	}
	for _, name := range []string{" * ", ".", "..", "音響\u0007映像"} {
		for _, request := range []struct{ method, path string }{
			{http.MethodPost, "/api/switches"}, {http.MethodPut, "/api/switches/1"},
		} {
			w := do(h, request.method, request.path, switchBody(name))
			if w.Code != http.StatusBadRequest || len(c.send) != 0 {
				t.Fatalf("無効なグループ名 %q: %d, 通知数 %d", name, w.Code, len(c.send))
			}
		}
	}
	for _, request := range []struct{ method, path, name, want string }{
		{http.MethodPost, "/api/switches", " 映像 ", "映像"},
		{http.MethodPut, "/api/switches/2", "   ", ""},
	} {
		w := do(h, request.method, request.path, switchBody(request.name))
		var saved model.SwitchConfig
		if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil || w.Code != http.StatusOK || saved.GroupTag != request.want {
			t.Fatalf("スイッチ保存: %d %s (%v)", w.Code, w.Body, err)
		}
		checkNotice()
	}
	w := do(h, http.MethodDelete, "/api/switches/2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("スイッチ削除: %d %s", w.Code, w.Body)
	}
	checkNotice()
}
