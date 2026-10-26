package api

import (
	"encoding/json"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/catalog"
	"kore-edge/internal/diagnostics"
	"kore-edge/internal/hls"
	"kore-edge/internal/integration"
	"kore-edge/internal/monitoring"
	"kore-edge/internal/multicast"
	"kore-edge/internal/provision"
	"kore-edge/internal/security"
	"kore-edge/internal/stream"
	"kore-edge/internal/webui"
	"kore-edge/internal/wireguard"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type API struct {
	WireGuardCredentials *wireguard.CredentialStore
	Mode                 string
	Accounts             *accounts.Store
	PublicOrigin         string
	Integration          *integration.Service
	Security             *security.Controller
	Provision            *provision.Registry
	ProvisionEdgeID      string
	Monitoring           *monitoring.Service
	HLS                  *hls.Manager
	MulticastInterfaces  func() ([]multicast.Interface, error)
	ready                atomic.Bool
	Catalog              *catalog.Store
	Streams              *stream.Engine
}

func (a *API) SetReady(ready bool) { a.ready.Store(ready) }

func (a *API) Handler() http.Handler {
	guard := a.Security
	if guard == nil {
		guard = security.Local()
	}
	handler := guard.Wrap(a.routes())
	if a.Accounts != nil {
		panel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/wireguard/setup" || r.URL.Path == "/api/v1/wireguard/setup/identity" || r.URL.Path == "/api/v1/wireguard/setup/peer" {
				a.wireGuardSetup(w, r)
				return
			}
			handler.ServeHTTP(w, r)
		})
		return a.Accounts.Wrap(a.PublicOrigin, panel)
	}
	return handler
}

func (a *API) routes() http.Handler {
	started := time.Now().UTC()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/provisioning" || r.URL.Path == "/api/v1/provisioning/register" {
			a.provisionHandler(w, r)
			return
		}
		if r.URL.Path == "/" || strings.HasPrefix(r.URL.Path, "/ui/") {
			webui.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/v1/integration" {
			a.integrationHandler(w, r)
			return
		}
		if r.URL.Path == "/api/v1/session" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(security.PrincipalFrom(r.Context()))
			return
		}
		if r.URL.Path == "/api/v1/edge" {
			a.edgeHandler(w, r, started)
			return
		}
		if r.URL.Path == "/metrics" || r.URL.Path == "/api/v1/monitoring" {
			a.monitoringHandler(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/hls/") || strings.HasPrefix(r.URL.Path, "/player/") || strings.HasPrefix(r.URL.Path, "/player-assets/") {
			if a.HLS == nil {
				w.WriteHeader(503)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/hls/") {
				a.HLS.ServeHTTP(w, r)
			} else {
				a.HLS.Player(w, r)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/api/v1/multicast/interfaces" || r.URL.Path == "/api/v1/multicast/plan" {
			a.multicastHandler(w, r)
			return
		}
		if r.URL.Path == "/api/v1/streams" || strings.HasPrefix(r.URL.Path, "/api/v1/streams/") {
			a.streamHandler(w, r)
			return
		}
		if r.URL.Path == "/api/v1/catalog" {
			a.catalogHandler(w, r)
			return
		}
		if r.URL.Path == "/api/v1/wireguard/plan" {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var request wireguard.Request
			d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
			d.DisallowUnknownFields()
			if d.Decode(&request) != nil || d.Decode(new(any)) != io.EOF {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"invalid plan request"}`))
				return
			}
			plan, err := wireguard.DryRun(request)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			_ = json.NewEncoder(w).Encode(plan)
			return
		}
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && r.URL.Path != "/api/v1/diagnostics" && r.URL.Path != "/api/v1/wireguard" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":"not_found"}`))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/api/v1/wireguard" {
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(wireguard.Collect(r.Context()))
			}
			return
		}
		if r.URL.Path == "/api/v1/diagnostics" {
			if r.Method == http.MethodGet {
				_ = json.NewEncoder(w).Encode(diagnostics.Collect())
			}
			return
		}
		code, body := http.StatusOK, `{"status":"ok","mode":"`+a.mode()+`"}`
		if r.URL.Path == "/readyz" && !a.ready.Load() {
			code, body = http.StatusServiceUnavailable, `{"status":"not_ready","mode":"`+a.mode()+`"}`
		}
		w.WriteHeader(code)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(body))
		}
	})
}

func (a *API) Server() *http.Server {
	return &http.Server{Handler: a.Handler(), ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 * 1024}
}

func (a *API) mode() string {
	if a.Mode == "production" {
		return "production"
	}
	return "laboratory"
}
