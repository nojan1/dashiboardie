package wol

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"dashiboardie/internal/service"
)

func TestParse(t *testing.T) {
	hosts, err := Parse([]string{
		"nas=AA-BB-CC-DD-EE-FF",
		"desktop=11:22:33:44:55:66@192.168.1.255",
		"laptop=11:22:33:44:55:77@192.168.1.255:7",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []Host{
		{"nas", "aa:bb:cc:dd:ee:ff", DefaultTarget},
		{"desktop", "11:22:33:44:55:66", "192.168.1.255:9"},
		{"laptop", "11:22:33:44:55:77", "192.168.1.255:7"},
	}
	for i := range want {
		if hosts[i] != want[i] {
			t.Errorf("host %d: got %+v, want %+v", i, hosts[i], want[i])
		}
	}
	for _, bad := range [][]string{
		{"nas"},
		{"=aa:bb:cc:dd:ee:ff"},
		{"nas=not-a-mac"},
		{"nas=00:00:00:00:fe:80:00:00:00:00:00:00:02:00:5e:10:00:00:00:01"}, // 20-byte IPoIB
		{"nas=aa:bb:cc:dd:ee:ff@no such host!:9"},
		{"a=aa:bb:cc:dd:ee:ff", "a=aa:bb:cc:dd:ee:00"},
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q): want error", bad)
		}
	}
}

func TestWakeSendsMagicPacket(t *testing.T) {
	ln, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	hosts, err := Parse([]string{"nas=aa:bb:cc:dd:ee:ff@" + ln.LocalAddr().String()})
	if err != nil {
		t.Fatal(err)
	}
	w := New(hosts)
	if err := w.Wake(context.Background(), "nas"); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 200)
	ln.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := ln.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 102 || !bytes.Equal(buf[:6], bytes.Repeat([]byte{0xff}, 6)) {
		t.Fatalf("bad packet: % x", buf[:n])
	}
	mac := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	for i := 6; i < n; i += 6 {
		if !bytes.Equal(buf[i:i+6], mac) {
			t.Fatalf("bad MAC repetition at %d: % x", i, buf[:n])
		}
	}

	if err := w.Wake(context.Background(), "aa:bb:cc:dd:ee:ff"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("unknown host: want ErrNotFound, got %v", err)
	}
}
