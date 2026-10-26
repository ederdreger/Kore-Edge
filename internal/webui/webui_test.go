package webui

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedDashboard(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/", 200}, {"HEAD", "/", 200}, {"POST", "/", 405}, {"GET", "/ui/", 200},
		{"GET", "/ui/assets/../index.html", 404}, {"GET", "/ui/.env", 404}, {"GET", "/ui/assets/not-found.js", 404},
	} {
		w := httptest.NewRecorder()
		ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
		if tc.code == 200 && w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("missing CSP")
		}
		if tc.code == 200 && (!strings.Contains(w.Header().Get("Content-Security-Policy"), "media-src 'self' blob:") || !strings.Contains(w.Header().Get("Content-Security-Policy"), "connect-src 'self';")) {
			t.Fatal("preview media must remain restricted to the local origin")
		}
	}
	assets, e := fs.ReadDir(files, "dist/assets")
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, asset := range assets {
		if !strings.HasSuffix(asset.Name(), ".js") {
			continue
		}
		found = true
		w := httptest.NewRecorder()
		ServeHTTP(w, httptest.NewRequest("GET", "/ui/assets/"+asset.Name(), nil))
		if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
			t.Fatal("asset unavailable")
		}
	}
	if !found {
		t.Fatal("frontend build missing")
	}
}
