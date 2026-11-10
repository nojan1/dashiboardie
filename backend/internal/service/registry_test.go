package service

import "testing"

func TestDedupeQuadlets(t *testing.T) {
	in := []Service{
		{ID: "jellyfin", Source: SourceQuadlet, Unit: "jellyfin.service"},
		{ID: "jellyfin.service", Source: SourceSystemd, Unit: "jellyfin.service"},
		{ID: "nginx.service", Source: SourceSystemd, Unit: "nginx.service"},
		{ID: "redis", Source: SourcePodman},
	}
	got := dedupeQuadlets(in)
	if len(got) != 3 {
		t.Fatalf("want 3 services, got %d: %+v", len(got), got)
	}
	for _, s := range got {
		if s.Source == SourceSystemd && s.Unit == "jellyfin.service" {
			t.Fatalf("systemd duplicate of quadlet was not removed")
		}
	}
}
