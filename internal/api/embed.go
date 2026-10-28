package api

import (
	_ "embed"
	"kore-edge/internal/hls"
	"net/http"
)

//go:embed embed.js
var embedScript []byte

func serveEmbed(w http.ResponseWriter, r *http.Request, tail string) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; media-src 'self' blob:; base-uri 'none'; frame-ancestors *; form-action 'none'")
	mime := "text/html; charset=utf-8"
	data := []byte(`<!doctype html><html lang="pt-BR"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Kore Edge · Canal ao vivo</title><style>html,body{margin:0;background:#0d1522;color:#d9e3f0;font:13px system-ui}video{width:100%;max-height:calc(100vh - 44px);aspect-ratio:16/9;display:block;background:#000}p{margin:8px 12px}</style><video controls playsinline muted aria-label="Canal ao vivo"></video><p role="status">Carregando saída HLS…</p><script src="hls.js"></script><script src="embed.js"></script></html>`)
	if tail == "embed.js" {
		mime = "text/javascript; charset=utf-8"
		data = embedScript
	}
	if tail == "hls.js" {
		mime = "text/javascript; charset=utf-8"
		data = hls.PlayerLibrary()
	}
	w.Header().Set("Content-Type", mime)
	if r.Method != "HEAD" {
		w.Write(data)
	}
}
