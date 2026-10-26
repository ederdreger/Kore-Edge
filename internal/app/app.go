// Package app owns the control API and optional synthetic stream lifecycle.
package app

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"kore-edge/internal/accounts"
	"kore-edge/internal/api"
	"kore-edge/internal/catalog"
	"kore-edge/internal/config"
	"kore-edge/internal/diagnostics"
	"kore-edge/internal/hls"
	"kore-edge/internal/integration"
	"kore-edge/internal/monitoring"
	"kore-edge/internal/provision"
	"kore-edge/internal/security"
	"kore-edge/internal/stream"
)

func Run(ctx context.Context, c config.Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if err := security.CheckPrivileges(); err != nil {
		return err
	}
	guard, err := security.Load(c.Security)
	if err != nil {
		return err
	}
	defer guard.Close()
	ln, err := net.Listen("tcp", c.HTTP.Listen)
	if err != nil {
		return err
	}
	var store *catalog.Store
	if c.Catalog.PartnerID != "" {
		store, err = catalog.New(c.Catalog.PartnerID, c.Catalog.PublicKey)
		if err != nil {
			ln.Close()
			return err
		}
	}
	var engine *stream.Engine
	var manager *hls.Manager
	if c.HLS.Enabled {
		manager, err = hls.New(ctx, c.HLS, func(id string, now time.Time) bool { return store.Authorize(id, "hls", now) })
		if err != nil {
			ln.Close()
			return err
		}
		defer manager.Close()
	}
	if c.Stream.Enabled {
		factory, err := stream.SyntheticFactory()
		if c.HLS.Enabled {
			factory = stream.AVFactory(c.HLS.FFmpeg)
			err = nil
		}
		if err != nil {
			ln.Close()
			return err
		}
		engine, err = stream.NewWithOutput(ctx, factory, func(id string, now time.Time) bool {
			if c.HLS.Enabled && store.Authorize(id, "hls", now) {
				return true
			}
			if c.Multicast.Mode == "send" {
				for _, t := range c.Multicast.Outputs {
					if t.ChannelID == id {
						return store.Authorize(id, "udp", now)
					}
				}
			}
			if c.HLS.Enabled {
				return false
			}
			return store.Authorize(id, "udp", now) || store.Authorize(id, "hls", now)
		}, stream.DefaultPolicy(), outputs(multicastOutput(c.Multicast, store), manager, store))
		if err != nil {
			ln.Close()
			return err
		}
		defer engine.Close()
	}
	var registry *provision.Registry
	if c.Provision.Enabled {
		registry, err = provision.Open(c.Provision.Directory)
		if err != nil {
			ln.Close()
			return err
		}
	}
	var synchronizer *integration.Service
	if c.Integration.Enabled {
		synchronizer, err = integration.NewMock(c.Integration, store)
		if err != nil {
			ln.Close()
			return err
		}
	}
	var users *accounts.Store
	if c.Accounts.Enabled {
		users, err = accounts.Open(c.Accounts.Directory)
		if err != nil {
			ln.Close()
			return err
		}
		defer users.Close()
		if len(users.Users()) == 0 {
			return accounts.ErrStorage
		}
	}
	return servePanel(ctx, ln, store, engine, manager, registry, c.Provision.EdgeID, guard, synchronizer, users, c.Accounts.PublicOrigin, c.Mode)
}

// Serve takes ownership of the listener, allowing tests to use ephemeral ports.
func Serve(ctx context.Context, ln net.Listener) error {
	return serve(ctx, ln, nil)
}

func serve(ctx context.Context, ln net.Listener, store *catalog.Store) error {
	return serveWithEngine(ctx, ln, store, nil)
}

func serveWithEngine(ctx context.Context, ln net.Listener, store *catalog.Store, engine *stream.Engine) error {
	return serveWithHLS(ctx, ln, store, engine, nil)
}
func serveWithHLS(ctx context.Context, ln net.Listener, store *catalog.Store, engine *stream.Engine, manager *hls.Manager) error {
	return serveWithProvision(ctx, ln, store, engine, manager, nil, "")
}
func serveWithProvision(ctx context.Context, ln net.Listener, store *catalog.Store, engine *stream.Engine, manager *hls.Manager, registry *provision.Registry, edgeID string) error {
	return serveSecure(ctx, ln, store, engine, manager, registry, edgeID, security.Local())
}
func serveSecure(ctx context.Context, ln net.Listener, store *catalog.Store, engine *stream.Engine, manager *hls.Manager, registry *provision.Registry, edgeID string, guard *security.Controller) error {
	return serveIntegrated(ctx, ln, store, engine, manager, registry, edgeID, guard, nil)
}
func serveIntegrated(ctx context.Context, ln net.Listener, store *catalog.Store, engine *stream.Engine, manager *hls.Manager, registry *provision.Registry, edgeID string, guard *security.Controller, synchronizer *integration.Service) error {
	return servePanel(ctx, ln, store, engine, manager, registry, edgeID, guard, synchronizer, nil, "", "laboratory")
}
func servePanel(ctx context.Context, ln net.Listener, store *catalog.Store, engine *stream.Engine, manager *hls.Manager, registry *provision.Registry, edgeID string, guard *security.Controller, synchronizer *integration.Service, users *accounts.Store, origin, mode string) error {
	if synchronizer != nil {
		syncCtx, stopSync := context.WithCancel(ctx)
		syncDone := make(chan struct{})
		go func() { defer close(syncDone); synchronizer.Run(syncCtx) }()
		defer func() { stopSync(); <-syncDone }()
	}
	ln = security.LimitListener(ln)
	defer ln.Close()
	var streams func() []stream.Status
	if engine != nil {
		streams = engine.Snapshot
	}
	observer := monitoring.New(streams, diagnostics.Network)
	monitorCtx, stopMonitor := context.WithCancel(ctx)
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); observer.Run(monitorCtx) }()
	defer func() { stopMonitor(); <-monitorDone }()
	a := &api.API{Mode: mode, Accounts: users, PublicOrigin: origin, Integration: synchronizer, Security: guard, Catalog: store, Streams: engine, HLS: manager, Monitoring: observer, Provision: registry, ProvisionEdgeID: edgeID}
	server := a.Server()
	server.TLSConfig = guard.TLSConfig()
	server.ErrorLog = log.New(io.Discard, "", 0)
	done := make(chan error, 1)
	a.SetReady(true)
	go func() {
		if server.TLSConfig != nil {
			done <- server.ServeTLS(ln, "", "")
		} else {
			done <- server.Serve(ln)
		}
	}()
	select {
	case err := <-done:
		a.SetReady(false)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		a.SetReady(false)
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := server.Shutdown(shutdown)
		if err != nil {
			_ = server.Close()
		}
		serveErr := <-done
		if err != nil {
			return err
		}
		if !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
}
