// Package web serves the built frontend with caching headers suited to a
// Nix store deployment.
//
// Files in the Nix store all have an mtime of 1970, so http.FileServer's
// Last-Modified header is useless: with no Cache-Control, browsers apply
// heuristic freshness (10% of the file's apparent age, i.e. years), and
// If-Modified-Since revalidation always gets a 304 even after a deploy.
//
// Instead:
//   - /assets/* (Vite output with content hashes in the file names) is
//     cached for a year as immutable;
//   - everything else (index.html, favicon) is served with no-cache and a
//     content-hash ETag, so the browser revalidates on every load and only
//     downloads the file again when it actually changed.
package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

const immutable = "public, max-age=31536000, immutable"

// Handler serves the frontend from dir.
func Handler(dir string) http.Handler {
	root := os.DirFS(dir)
	assets := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", immutable)
			assets.ServeHTTP(w, r)
			return
		}
		serveRevalidated(w, r, root)
	})
}

func serveRevalidated(w http.ResponseWriter, r *http.Request, root fs.FS) {
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	data, err := fs.ReadFile(root, name)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		// A directory, or unreadable: fall back to the index page check.
		if info, serr := fs.Stat(root, name); serr == nil && info.IsDir() {
			data, err = fs.ReadFile(root, path.Join(name, "index.html"))
			name = path.Join(name, "index.html")
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}
	sum := sha256.Sum256(data)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
	// A zero modtime omits Last-Modified, so only the ETag is used to
	// revalidate. ServeContent handles If-None-Match, HEAD and ranges.
	http.ServeContent(w, r, name, time.Time{}, io.ReadSeeker(bytes.NewReader(data)))
}
