// Package wol sends Wake-on-LAN magic packets to a configured list of hosts.
//
// The API is unauthenticated, so clients only ever name a configured host;
// the MAC address and target address come from the configuration.
package wol

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"

	"dashiboardie/internal/service"
)

// DefaultTarget is the limited broadcast address on the discard port, which
// leaves through the interface of the default route.
const DefaultTarget = "255.255.255.255:9"

type Host struct {
	Name string `json:"name"`
	MAC  string `json:"mac"`
	// Target is where the packet is sent: a broadcast address with a port.
	Target string `json:"target"`
}

// Parse reads specs of the form name=mac or name=mac@address[:port], e.g.
// nas=aa:bb:cc:dd:ee:ff@192.168.1.255. The port defaults to 9.
func Parse(specs []string) ([]Host, error) {
	var hosts []Host
	seen := map[string]bool{}
	for _, spec := range specs {
		name, rest, ok := strings.Cut(spec, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("wake-on-lan host %q: want name=mac[@address]", spec)
		}
		if seen[name] {
			return nil, fmt.Errorf("wake-on-lan host %q listed twice", name)
		}
		seen[name] = true
		mac, target, _ := strings.Cut(strings.TrimSpace(rest), "@")
		hw, err := net.ParseMAC(mac)
		if err != nil || len(hw) != 6 {
			return nil, fmt.Errorf("wake-on-lan host %q: bad MAC address %q", name, mac)
		}
		switch {
		case target == "":
			target = DefaultTarget
		case !hasPort(target):
			target = net.JoinHostPort(strings.Trim(target, "[]"), "9")
		}
		if _, err := net.ResolveUDPAddr("udp", target); err != nil {
			return nil, fmt.Errorf("wake-on-lan host %q: bad address %q: %w", name, target, err)
		}
		hosts = append(hosts, Host{Name: name, MAC: hw.String(), Target: target})
	}
	return hosts, nil
}

func hasPort(addr string) bool {
	_, _, err := net.SplitHostPort(addr)
	return err == nil
}

// MagicPacket is 6 bytes of 0xff followed by the MAC address 16 times.
func MagicPacket(mac net.HardwareAddr) []byte {
	return append(bytes.Repeat([]byte{0xff}, 6), bytes.Repeat(mac, 16)...)
}

type Waker struct {
	hosts []Host
	// Send delivers a packet; tests and -mock replace it.
	Send func(ctx context.Context, target string, packet []byte) error
}

func New(hosts []Host) *Waker {
	return &Waker{hosts: hosts, Send: sendUDP}
}

func (w *Waker) Hosts() []Host {
	if w == nil || w.hosts == nil {
		return []Host{}
	}
	return w.hosts
}

// Wake sends the magic packet for the named host.
func (w *Waker) Wake(ctx context.Context, name string) error {
	for _, h := range w.Hosts() {
		if h.Name == name {
			mac, err := net.ParseMAC(h.MAC)
			if err != nil {
				return err
			}
			return w.Send(ctx, h.Target, MagicPacket(mac))
		}
	}
	return service.ErrNotFound
}

// sendUDP sends the packet a few times, since UDP may drop one. Go sets
// SO_BROADCAST on UDP sockets, so broadcast targets work.
func sendUDP(ctx context.Context, target string, packet []byte) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", target)
	if err != nil {
		return err
	}
	defer conn.Close()
	for range 3 {
		if _, err := conn.Write(packet); err != nil {
			return err
		}
	}
	return nil
}
