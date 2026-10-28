package manualchannels

import (
	"context"
	srt "github.com/datarhei/gosrt"
	"io"
	"kore-edge/internal/edgecatalog"
	"kore-edge/internal/multicast"
	"net"
	"net/http"
	"os/exec"
)

type Catalog interface {
	Snapshot() edgecatalog.Snapshot
	Authorized(string, string) bool
	OutputAuthorized(string, string) bool
	OpenHTTP(context.Context, string) (io.ReadCloser, error)
	OpenHLS(context.Context, string, string) (io.ReadCloser, error)
	OpenSRT(context.Context, string) (io.ReadCloser, error)
}
type Service struct {
	protected
	directory      string
	data           state
	presentation   presentationState
	processing     processingState
	nextInput      map[string]int
	activeInput    map[string]string
	encoders       encoderCache
	transcodeSlots chan struct{}
	fallback       Catalog
	client         *http.Client
	lookup         func(context.Context, string) ([]net.IPAddr, error)
	interfaces     func() ([]multicast.Interface, error)
	slots          chan struct{}
	dialSRT        func(string, string, srt.Config) (srt.Conn, error)
	command        func(context.Context, string, ...string) *exec.Cmd
}

func (s *Service) Close() { s.client.CloseIdleConnections() }

func (s *Service) Snapshot() edgecatalog.Snapshot {
	var out edgecatalog.Snapshot
	if s.fallback != nil {
		out = s.fallback.Snapshot()
	}
	s.mu.Lock()
	sources := append([]Source{}, s.data.Sources...)
	for i := range sources {
		sources[i].Name = s.displayName(sources[i])
	}
	s.mu.Unlock()
	if len(sources) == 0 {
		return out
	}
	if out.Catalog == nil {
		out.Catalog = &edgecatalog.Manifest{Version: 1, Channels: []edgecatalog.Channel{}}
	} else {
		copy := *out.Catalog
		copy.Channels = []edgecatalog.Channel{}
		for _, c := range out.Catalog.Channels {
			if !s.IsManual(c.ID) {
				if !out.Available {
					c.Protocols = nil
					c.Outputs = nil
				}
				copy.Channels = append(copy.Channels, c)
			}
		}
		out.Catalog = &copy
	}
	for _, c := range sources {
		duplicate := false
		for _, remote := range out.Catalog.Channels {
			if remote.ID == c.ID {
				duplicate = true
			}
		}
		if duplicate {
			continue
		}
		entry := edgecatalog.Channel{ID: c.ID, Name: c.Name, Category: c.Category}
		if c.Enabled {
			entry.Outputs = []string{"udp", "hls"}
			entry.Protocols = []edgecatalog.Protocol{{ID: c.Protocol, Label: protocolLabel(c.Protocol), Verification: "not_tested"}}
		}
		out.Catalog.Channels = append(out.Catalog.Channels, entry)
	}
	// This is a local combined view, not an extension of a remote signed lease.
	// Remote authorization still delegates to the original catalog on every use.
	out.Available = out.Available || len(out.Catalog.Channels) > 0
	return out
}
func protocolLabel(p string) string {
	switch p {
	case "http_ts":
		return "HTTP MPEG-TS"
	case "hls":
		return "HLS"
	case "srt":
		return "SRT"
	case "udp":
		return "UDP MPEG-TS"
	}
	return "Indisponível"
}
func (s *Service) IsManual(id string) bool { return localID.MatchString(id) }
func (s *Service) Authorized(id, protocol string) bool {
	if s.IsManual(id) {
		c, ok := s.source(id)
		return ok && c.Enabled && c.Protocol == protocol
	}
	return s.fallback != nil && s.fallback.Authorized(id, protocol)
}
func (s *Service) OutputAuthorized(id, output string) bool {
	if s.IsManual(id) {
		c, ok := s.source(id)
		return ok && c.Enabled && (output == "udp" || output == "hls")
	}
	return s.fallback != nil && s.fallback.OutputAuthorized(id, output)
}
func (s *Service) OpenHTTP(ctx context.Context, id string) (io.ReadCloser, error) {
	if !s.IsManual(id) {
		if s.fallback == nil {
			return nil, ErrDenied
		}
		return s.fallback.OpenHTTP(ctx, id)
	}
	c, ok := s.source(id)
	if !ok || !s.Authorized(id, "http_ts") {
		return nil, ErrDenied
	}
	r, e := s.openHTTP(ctx, c.URL, "")
	if e != nil {
		return nil, e
	}
	return r.Body, nil
}
func (s *Service) OpenHLS(ctx context.Context, id, ffmpeg string) (io.ReadCloser, error) {
	if !s.IsManual(id) {
		if s.fallback == nil {
			return nil, ErrDenied
		}
		return s.fallback.OpenHLS(ctx, id, ffmpeg)
	}
	c, ok := s.source(id)
	if !ok || !s.Authorized(id, "hls") {
		return nil, ErrDenied
	}
	return s.openHLS(ctx, c, ffmpeg)
}
func (s *Service) OpenSRT(ctx context.Context, id string) (io.ReadCloser, error) {
	if !s.IsManual(id) {
		if s.fallback == nil {
			return nil, ErrDenied
		}
		return s.fallback.OpenSRT(ctx, id)
	}
	c, ok := s.source(id)
	if !ok || !s.Authorized(id, "srt") {
		return nil, ErrDenied
	}
	return s.openSRT(ctx, c)
}
func (s *Service) OpenUDP(ctx context.Context, id string) (io.ReadCloser, error) {
	c, ok := s.source(id)
	if !ok || !s.Authorized(id, "udp") {
		return nil, ErrDenied
	}
	return s.openUDP(ctx, c)
}
