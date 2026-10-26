// Package webui serves only the built, embedded laboratory dashboard.
package webui

import (
	"bytes"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed dist
var files embed.FS

func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(405)
		return
	}
	name := "dist/index.html"
	if r.URL.Path != "/" && r.URL.Path != "/ui/" {
		if !strings.HasPrefix(r.URL.Path, "/ui/assets/") && !strings.HasPrefix(r.URL.Path, "/ui/licenses/") {
			w.WriteHeader(404)
			return
		}
		relative := strings.TrimPrefix(r.URL.Path, "/ui/")
		if !fs.ValidPath(relative) || strings.Contains(relative, "\\") || path.Clean(relative) != relative {
			w.WriteHeader(404)
			return
		}
		name = "dist/" + relative
	}
	data, e := files.ReadFile(name)
	if e != nil {
		w.WriteHeader(404)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasPrefix(name, "dist/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	content := mime.TypeByExtension(path.Ext(name))
	if content == "" {
		content = "application/octet-stream"
	}
	w.Header().Set("Content-Type", content)
	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(data))
}
