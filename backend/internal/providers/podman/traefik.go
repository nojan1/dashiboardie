package podman

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
)

var (
	hostRule   = regexp.MustCompile("Host\\(\\s*`([^`]+)`")
	prefixRule = regexp.MustCompile("PathPrefix\\(\\s*`([^`]+)`")
)

// traefikURL derives a link from Traefik docker-provider labels, e.g.
// traefik.http.routers.bazarr.rule=Host(`bazarr.home`). It returns "" when
// no router has a Host() rule or the container opts out with
// traefik.enable=false.
//
// The scheme is https if the router has TLS configured or uses an
// entrypoint whose name suggests TLS (websecure, https, ...), else http.
// Routers are tried in name order, preferring https ones.
func traefikURL(labels map[string]string) string {
	if strings.EqualFold(labels["traefik.enable"], "false") {
		return ""
	}
	type candidate struct {
		url, router string
		https       bool
	}
	var found []candidate
	for key, rule := range labels {
		router, ok := strings.CutPrefix(key, "traefik.http.routers.")
		if !ok {
			continue
		}
		router, ok = strings.CutSuffix(router, ".rule")
		if !ok {
			continue
		}
		m := hostRule.FindStringSubmatch(rule)
		if m == nil {
			continue
		}
		path := ""
		if p := prefixRule.FindStringSubmatch(rule); p != nil {
			path = p[1]
		}
		https := routerUsesTLS(labels, router)
		scheme := "http"
		if https {
			scheme = "https"
		}
		found = append(found, candidate{scheme + "://" + m[1] + path, router, https})
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].https != found[j].https {
			return found[i].https
		}
		return found[i].router < found[j].router
	})
	return found[0].url
}

func routerUsesTLS(labels map[string]string, router string) bool {
	prefix := "traefik.http.routers." + router + "."
	if strings.EqualFold(labels[prefix+"tls"], "true") || labels[prefix+"tls.certresolver"] != "" {
		return true
	}
	for _, ep := range strings.Split(labels[prefix+"entrypoints"], ",") {
		ep = strings.ToLower(strings.TrimSpace(ep))
		if strings.Contains(ep, "secure") || strings.Contains(ep, "https") || strings.Contains(ep, "443") || ep == "tls" {
			return true
		}
	}
	return false
}

// quadletLabels reads the Label= and PublishPort= entries from the [X-Container] section that
// Quadlet copies into the generated unit. This works while the Quadlet is
// stopped (and its container removed), unlike `podman ps`.
func quadletLabels(unitPath string) (map[string]string, []string, error) {
	f, err := os.Open(unitPath)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	labels := map[string]string{}
	var publish []string
	inContainer := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			inContainer = line == "[X-Container]"
			continue
		}
		if !inContainer {
			continue
		}
		if p, ok := strings.CutPrefix(line, "PublishPort="); ok {
			publish = append(publish, p)
			continue
		}
		v, ok := strings.CutPrefix(line, "Label=")
		if !ok {
			continue
		}
		for _, kv := range splitLabelValue(v) {
			if k, val, ok := strings.Cut(kv, "="); ok {
				labels[k] = val
			}
		}
	}
	return labels, publish, sc.Err()
}

// splitLabelValue handles both forms Quadlet accepts:
//
//	Label=key=value with spaces
//	Label="a=1" "b=2"
func splitLabelValue(v string) []string {
	if !strings.HasPrefix(v, `"`) {
		return []string{v}
	}
	var out []string
	for v = strings.TrimSpace(v); v != ""; v = strings.TrimSpace(v) {
		if !strings.HasPrefix(v, `"`) {
			out = append(out, v)
			break
		}
		end := strings.Index(v[1:], `"`)
		if end < 0 {
			out = append(out, v[1:])
			break
		}
		out = append(out, v[1:end+1])
		v = v[end+2:]
	}
	return out
}
