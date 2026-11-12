package podman

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTraefikURL(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name: "plain http entrypoint (as on the test VM)",
			labels: map[string]string{
				"traefik.enable": "true",
				"traefik.http.routers.bazarr.entrypoints":               "web",
				"traefik.http.routers.bazarr.rule":                      "Host(`bazarr.home`)",
				"traefik.http.services.bazarr.loadbalancer.server.port": "6767",
			},
			want: "http://bazarr.home",
		},
		{
			name: "tls and path prefix",
			labels: map[string]string{
				"traefik.http.routers.app.rule": "Host(`example.com`) && PathPrefix(`/app`)",
				"traefik.http.routers.app.tls":  "true",
			},
			want: "https://example.com/app",
		},
		{
			name: "websecure entrypoint and multiple hosts",
			labels: map[string]string{
				"traefik.http.routers.a.rule":        "Host(`a.example.com`) || Host(`b.example.com`)",
				"traefik.http.routers.a.entrypoints": "websecure",
			},
			want: "https://a.example.com",
		},
		{
			name: "https router preferred over http redirect router",
			labels: map[string]string{
				"traefik.http.routers.a-http.rule":              "Host(`x.example.com`)",
				"traefik.http.routers.a-http.entrypoints":       "web",
				"traefik.http.routers.a-https.rule":             "Host(`x.example.com`)",
				"traefik.http.routers.a-https.tls.certresolver": "le",
			},
			want: "https://x.example.com",
		},
		{
			name: "disabled",
			labels: map[string]string{
				"traefik.enable":              "false",
				"traefik.http.routers.a.rule": "Host(`a.home`)",
			},
			want: "",
		},
		{
			name:   "no rule",
			labels: map[string]string{"PODMAN_SYSTEMD_UNIT": "x.service"},
			want:   "",
		},
		{
			name:   "nil labels",
			labels: nil,
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := traefikURL(tt.labels); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuadletLabels(t *testing.T) {
	// Layout of a real generated unit: [X-Container] holds the original
	// .container keys; other sections must be ignored.
	unit := "[X-Container]\n" +
		"Image=lscr.io/linuxserver/bazarr\n" +
		"Label=traefik.enable=true\n" +
		"Label=traefik.http.routers.bazarr.rule=Host(`bazarr.home`)\n" +
		"Label=\"a=1\" \"b=two words\"\n" +
		"\n[Unit]\n" +
		"Label=not.a.container.label=1\n" +
		"SourcePath=/etc/containers/systemd/bazarr.container\n"
	path := filepath.Join(t.TempDir(), "bazarr.service")
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		t.Fatal(err)
	}
	labels, _, err := quadletLabels(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"traefik.enable":                   "true",
		"traefik.http.routers.bazarr.rule": "Host(`bazarr.home`)",
		"a":                                "1",
		"b":                                "two words",
	}
	if len(labels) != len(want) {
		t.Fatalf("got %v, want %v", labels, want)
	}
	for k, v := range want {
		if labels[k] != v {
			t.Errorf("%s: got %q, want %q", k, labels[k], v)
		}
	}
}
