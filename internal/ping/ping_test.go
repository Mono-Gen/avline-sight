package ping

import (
	"context"
	"testing"
	"time"
)

// 自ホストは必ず応答し、TEST-NET-1（RFC 5737, 192.0.2.0/24）は経路上に存在しないため応答しない。
func TestLoopbackAndUnreachable(t *testing.T) {
	rs := Many(context.Background(), []string{"127.0.0.1", "192.0.2.1", "not-an-ip"}, 500*time.Millisecond)
	if rs[0].IP != "127.0.0.1" || rs[0].Status != StatusOK || rs[0].RTTms < 0 {
		t.Fatalf("ループバック: %+v", rs[0])
	}
	if rs[1].Status == StatusOK {
		t.Fatalf("TEST-NETが応答扱い: %+v", rs[1])
	}
	if rs[2].Status != StatusError {
		t.Fatalf("不正IP: %+v", rs[2])
	}
}

func TestManyBoundedParallel(t *testing.T) {
	ips := make([]string, 40)
	for i := range ips {
		ips[i] = "127.0.0.1"
	}
	start := time.Now()
	rs := Many(context.Background(), ips, time.Second)
	for _, r := range rs {
		if r.Status != StatusOK {
			t.Fatalf("%+v", r)
		}
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("並列実行されていない")
	}
}
