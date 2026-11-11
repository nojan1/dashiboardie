package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newSite mimics a Nix store deployment: every file has a 1970 mtime.
func newSite(t *testing.T, index string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"index.html":           index,
		"favicon.svg":          "<svg/>",
		"assets/index-abc.js":  "console.log(1)",
		"assets/index-abc.css": "body{}",
	}
	epoch := time.Unix(1, 0)
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, epoch, epoch); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func get(t *testing.T, h http.Handler, path string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIndexIsRevalidatedByContent(t *testing.T) {
	dir := newSite(t, "<title>v1</title>")
	h := Handler(dir)

	res := get(t, h, "/", nil)
	if res.Code != 200 || res.Body.String() != "<title>v1</title>" {
		t.Fatalf("got %d %q", res.Code, res.Body.String())
	}
	if cc := res.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if lm := res.Header().Get("Last-Modified"); lm != "" {
		t.Errorf("Last-Modified should be omitted, got %q", lm)
	}
	etag := res.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing ETag")
	}

	// Unchanged: 304.
	if res := get(t, h, "/", map[string]string{"If-None-Match": etag}); res.Code != http.StatusNotModified {
		t.Fatalf("want 304 for unchanged index, got %d", res.Code)
	}

	// "Deploy" a new version with the same 1970 mtime: must not be a 304.
	dir2 := newSite(t, "<title>v2</title>")
	res = get(t, Handler(dir2), "/", map[string]string{
		"If-None-Match":     etag,
		"If-Modified-Since": "Thu, 01 Jan 1970 00:00:01 GMT",
	})
	if res.Code != 200 || res.Body.String() != "<title>v2</title>" {
		t.Fatalf("new version should be served, got %d %q", res.Code, res.Body.String())
	}
}

func TestAssetsAreImmutable(t *testing.T) {
	h := Handler(newSite(t, "x"))
	res := get(t, h, "/assets/index-abc.js", nil)
	if res.Code != 200 || res.Body.String() != "console.log(1)" {
		t.Fatalf("got %d %q", res.Code, res.Body.String())
	}
	if cc := res.Header().Get("Cache-Control"); cc != immutable {
		t.Errorf("Cache-Control = %q", cc)
	}
}

func TestOtherFilesAndErrors(t *testing.T) {
	h := Handler(newSite(t, "x"))
	if res := get(t, h, "/favicon.svg", nil); res.Code != 200 || res.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("favicon: %d %q", res.Code, res.Header().Get("Cache-Control"))
	}
	if res := get(t, h, "/missing.txt", nil); res.Code != http.StatusNotFound {
		t.Errorf("missing file: %d", res.Code)
	}
	if res := get(t, h, "/../../etc/passwd", nil); res.Code != http.StatusNotFound {
		t.Errorf("path traversal: %d", res.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST: %d", rec.Code)
	}
}
