package mock

import (
	"context"
	"log/slog"

	"dashiboardie/internal/wol"
)

// Waker offers a few fake Wake-on-LAN hosts and only logs instead of sending.
func Waker() *wol.Waker {
	hosts, _ := wol.Parse([]string{
		"nas=aa:bb:cc:dd:ee:01",
		"gaming-pc=aa:bb:cc:dd:ee:02@192.168.1.255",
		"backup-box=aa:bb:cc:dd:ee:03",
	})
	w := wol.New(hosts)
	w.Send = func(_ context.Context, target string, _ []byte) error {
		slog.Info("mock: would send wake-on-lan packet", "target", target)
		return nil
	}
	return w
}
