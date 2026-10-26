package edgecatalog

import (
	"context"
	"io"
	"time"
)

// Authorized is shared by the real ingest and local output managers. It never
// expands a source grant into a redistribution grant.
func (s *Service) Authorized(id, protocol string) bool { return s.allowed(id, protocol) }
func (s *Service) OutputAuthorized(id, output string) bool {
	identity, e := s.credentials.Snapshot()
	if e != nil || identity.Settings == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.data
	if data.Catalog == nil || !time.Now().Before(data.Catalog.ExpiresAt) || identity.PublicKey != data.EdgeKey || identity.Settings.KorePlayPublicKey != data.MatrixKey {
		return false
	}
	for _, channel := range data.Catalog.Channels {
		if channel.ID == id {
			for _, allowed := range channel.Outputs {
				if allowed == output {
					return true
				}
			}
		}
	}
	return false
}
func (s *Service) OpenHTTP(ctx context.Context, id string) (io.ReadCloser, error) {
	if !s.allowed(id, "http_ts") {
		return nil, ErrDenied
	}
	response, _, e := s.remote(ctx, "/api/kore-edge/media/"+id+"/http_ts/stream.ts")
	if e != nil {
		return nil, e
	}
	return response.Body, nil
}
