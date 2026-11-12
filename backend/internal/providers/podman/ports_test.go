package podman

import (
	"slices"
	"testing"
)

func TestPublishPorts(t *testing.T) {
	got := publishPorts([]string{
		"8080:80",             // host port
		"8080:8080",           // duplicate
		"0.0.0.0:9000:9000",   // explicit wildcard
		"127.0.0.1:9001:9001", // loopback: unreachable
		"[::1]:9002:9002",     // loopback v6
		"[::]:9003:9003",      // wildcard v6
		"192.168.1.5:9004:80", // specific address
		"6881:6881/udp",       // not tcp
		"6882:6882/tcp",       // explicit tcp
		"7000-7002:7000-7002", // range: first port
		"3000",                // random host port
		"127.0.0.1::3001",     // random host port on loopback
		"garbage:x",
	})
	want := []int{8080, 9000, 9003, 9004, 6882, 7000}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestPsPorts(t *testing.T) {
	got := psPorts([]psPort{
		{HostIP: "", HostPort: 8096, Protocol: "tcp"},
		{HostIP: "", HostPort: 8096, Protocol: "tcp"},
		{HostIP: "127.0.0.1", HostPort: 5432, Protocol: "tcp"},
		{HostIP: "", HostPort: 51820, Protocol: "udp"},
		{HostIP: "0.0.0.0", HostPort: 3000, Protocol: "tcp"},
	})
	if want := []int{8096, 3000}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
