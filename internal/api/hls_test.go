package api

import (
	"net/http/httptest"
	"testing"
)

func TestHLSDisabled(t *testing.T) {
	for _, path := range []string{"/hls/test/index.m3u8", "/player/test", "/player-assets/player.js"} {
		w := httptest.NewRecorder()
		(&API{}).routes().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 503 {
			t.Fatal("disabled HLS")
		}
	}
}
