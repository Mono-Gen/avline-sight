package poller

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Mono-Gen/avline-sight/internal/mib"
	"github.com/Mono-Gen/avline-sight/internal/sim"
	sx "github.com/Mono-Gen/avline-sight/internal/snmpx"
)

// 診断用の部分木読み: 上限で打ち切り、部分木の外の値は返さない。
func TestWalkSubtreeStopsAtLimitAndRoot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lab := sim.NewLab(1, 1)
	if err := lab.Start(ctx, sim.Config{BaseSNMPPort: 27197}); err != nil {
		t.Fatal(err)
	}
	c, err := sx.Dial("127.0.0.1", 27197, "public", time.Second, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	part, truncated, err := c.WalkSubtree(mib.IfName, 5)
	if err != nil || !truncated || len(part) != 5 {
		t.Fatalf("limit 5: %d values, truncated=%v, err=%v", len(part), truncated, err)
	}
	all, truncated, err := c.WalkSubtree(mib.IfName, 1000)
	if err != nil || truncated || len(all) < 30 {
		t.Fatalf("whole column: %d values, truncated=%v, err=%v", len(all), truncated, err)
	}
	for _, p := range all {
		if !strings.HasPrefix(strings.TrimPrefix(p.Name, "."), mib.IfName+".") {
			t.Fatalf("value outside the subtree: %s", p.Name)
		}
	}
	if none, _, err := c.WalkSubtree("1.3.6.1.2.1.185", 100); err != nil || len(none) != 0 {
		t.Fatalf("an empty subtree returned %d values (err %v)", len(none), err)
	}
}
