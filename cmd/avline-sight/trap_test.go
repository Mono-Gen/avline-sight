package main

import (
	"github.com/Mono-Gen/avline-sight/internal/model"
	"testing"
)

func TestResolveTrapSourceChecksIPBeforeLabPort(t *testing.T) {
	sws := []model.SwitchConfig{{ID: 1, IP: "127.0.0.1", SNMPPort: 1161}, {ID: 2, IP: "127.0.0.1", SNMPPort: 1162}, {ID: 3, IP: "192.0.2.10", SNMPPort: 161}}
	if _, _, ok := resolveTrapSource(sws, "192.0.2.20", 1162, true); ok {
		t.Fatal("未登録の送信元がラボのスイッチとして解決された")
	}
	if id, _, ok := resolveTrapSource(sws, "127.0.0.1", 1162, true); !ok || id != 2 {
		t.Fatalf("ラボの送信元: %d %v", id, ok)
	}
	if id, _, ok := resolveTrapSource(sws, "192.0.2.10", 1162, false); !ok || id != 3 {
		t.Fatalf("登録された実機の送信元: %d %v", id, ok)
	}
}
