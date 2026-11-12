package podman

import (
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// Published ports are offered as links (http://<dashboard host>:<port>) for
// containers without a Traefik link. Only TCP ports bound to every address or
// a non-loopback one count: a port on 127.0.0.1 isn't reachable from the
// browser. A range contributes its first port.

// psPort is an entry of `podman ps --format json` "Ports".
type psPort struct {
	HostIP   string `json:"host_ip"`
	HostPort int    `json:"host_port"`
	Protocol string `json:"protocol"`
}

func psPorts(ports []psPort) []int {
	var out []int
	for _, p := range ports {
		if p.Protocol != "" && p.Protocol != "tcp" {
			continue
		}
		out = addPort(out, p.HostIP, p.HostPort)
	}
	return out
}

// publishPorts parses Quadlet PublishPort= values, which use podman's
// --publish syntax: [[ip:][hostPort]:]containerPort[/protocol], where the
// ports may be ranges and an IPv6 ip is in brackets. A value without a host
// port gets a random one, which can't be known from the unit file.
func publishPorts(values []string) []int {
	var out []int
	for _, v := range values {
		v, proto, _ := strings.Cut(strings.TrimSpace(v), "/")
		if proto != "" && proto != "tcp" {
			continue
		}
		ip := ""
		if strings.HasPrefix(v, "[") {
			end := strings.Index(v, "]")
			if end < 0 {
				continue
			}
			ip, v = v[1:end], strings.TrimPrefix(v[end+1:], ":")
		}
		parts := strings.Split(v, ":")
		var host string
		switch len(parts) {
		case 2: // hostPort:containerPort, or [ip]:hostPort:containerPort
			host = parts[0]
		case 3: // ip:hostPort:containerPort
			ip, host = parts[0], parts[1]
		default:
			continue
		}
		first, _, _ := strings.Cut(host, "-")
		n, err := strconv.Atoi(first)
		if err != nil {
			continue
		}
		out = addPort(out, ip, n)
	}
	return out
}

func addPort(ports []int, ip string, port int) []int {
	if port <= 0 || port > 65535 || slices.Contains(ports, port) {
		return ports
	}
	if a, err := netip.ParseAddr(ip); err == nil && a.IsLoopback() {
		return ports
	}
	return append(ports, port)
}
