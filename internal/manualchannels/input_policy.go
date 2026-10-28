package manualchannels

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
)

type InputPolicy struct {
	Publication     bool   `json:"publication"`
	ClusterExport   bool   `json:"cluster_export"`
	Token           string `json:"token,omitempty"`
	BackupAsset     string `json:"backup_asset,omitempty"`
	BackupTimeout   int    `json:"backup_timeout,omitempty"`
	VideoTimeout    int    `json:"video_timeout,omitempty"`
	AudioTimeout    int    `json:"audio_timeout,omitempty"`
	BackupTranscode bool   `json:"backup_transcode,omitempty"`
	RecordBackup    bool   `json:"record_backup,omitempty"`
	RotateToken     bool   `json:"rotate_token,omitempty"`
}

func (s *Service) openInput(ctx context.Context, id string, in Input, ffmpeg string) (io.ReadCloser, error) {
	candidate := Source{URL: in.URL, Protocol: in.Protocol, Interface: in.Interface, LocalIP: in.LocalIP}
	var r io.ReadCloser
	var e error
	switch in.Protocol {
	case "publish":
		r, e = s.openPublication(ctx, id)
	case "cluster":
		r, e = s.openCluster(ctx, in)
	case "http_ts":
		var response *http.Response
		response, e = s.openHTTP(ctx, in.URL, "")
		if e == nil {
			r = response.Body
		}
	case "hls":
		r, e = s.openHLS(ctx, candidate, ffmpeg)
	case "srt":
		if ffmpeg != "" {
			r, e = s.openNativeSRT(ctx, candidate, ffmpeg)
		} else {
			r, e = s.openSRT(ctx, candidate)
		}
	case "udp":
		r, e = s.openUDP(ctx, candidate)
	default:
		e = ErrInvalid
	}
	if e == nil {
		r = watchPackets(ctx, r, in.SourceTimeout, 0, 0)
	}
	return r, e
}
func validPolicy(p InputPolicy) bool {
	validTime := func(v int) bool { return v == 0 || v >= 2 && v <= 300 }
	return validTime(p.BackupTimeout) && validTime(p.VideoTimeout) && validTime(p.AudioTimeout) && (p.BackupAsset == "" || assetID.MatchString(p.BackupAsset) && strings.HasSuffix(p.BackupAsset, ".mp4")) && (p.Token == "" || inputKey.MatchString(p.Token) && p.Token != "primary")
}
func normalizeInput(v Input) (Input, error) {
	if v.SourceTimeout != 0 && (v.SourceTimeout < 2 || v.SourceTimeout > 300) {
		return v, ErrInvalid
	}
	if v.Protocol == "publish" {
		if v.URL != "publish://incoming" || v.Token != "" || v.Certificate != "" || v.Interface != "" || v.LocalIP != "" {
			return v, ErrInvalid
		}
		return v, nil
	}
	protocol := v.Protocol
	if protocol == "cluster" {
		u, e := url.Parse(v.URL)
		if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/api/v1/cluster/") || !strings.HasSuffix(u.Path, "/stream.ts") || !inputKey.MatchString(v.Token) || v.Token == "primary" || v.Certificate != "" && !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(v.Certificate) {
			return v, ErrInvalid
		}
		protocol = "http_ts"
	} else if v.Token != "" || v.Certificate != "" {
		return v, ErrInvalid
	}
	n, e := normalize(Source{Name: "Input", URL: v.URL, Protocol: protocol, Interface: v.Interface, LocalIP: v.LocalIP})
	if e != nil {
		return v, e
	}
	v.URL = n.URL
	if v.Protocol != "cluster" {
		v.Protocol = n.Protocol
	}
	return v, nil
}
func (s *Service) policyAuthorized(id, token string, publish bool) bool {
	c, ok := s.source(id)
	if !ok || !c.Enabled {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.processingLocked(id, c).Policy
	if !publish {
		for _, in := range s.processingLocked(id, c).Inputs {
			if in.Protocol == "cluster" {
				return false
			}
		}
	}
	enabled := p.ClusterExport
	if publish {
		enabled = p.Publication
	}
	return enabled && len(token) == 48 && subtle.ConstantTimeCompare([]byte(token), []byte(p.Token)) == 1
}
func (s *Service) IngestAuthorized(id, token string, publish bool) bool {
	return s.policyAuthorized(id, token, publish)
}

type publicationPipe struct {
	r         *io.PipeReader
	w         *io.PipeWriter
	publisher bool
	receiver  bool
}
type publicationReader struct {
	*io.PipeReader
	s    *Service
	id   string
	p    *publicationPipe
	once sync.Once
}

func (r *publicationReader) Close() error {
	r.once.Do(func() {
		r.s.pushMu.Lock()
		if r.s.pushes[r.id] == r.p {
			delete(r.s.pushes, r.id)
		}
		r.s.pushMu.Unlock()
		r.p.w.Close()
		r.PipeReader.Close()
	})
	return nil
}
func (s *Service) openPublication(ctx context.Context, id string) (io.ReadCloser, error) {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	if s.pushes == nil {
		s.pushes = map[string]*publicationPipe{}
	}
	if p := s.pushes[id]; p != nil {
		if p.receiver {
			return nil, ErrConflict
		}
		p.receiver = true
		return contextual(ctx, &publicationReader{PipeReader: p.r, s: s, id: id, p: p}), nil
	}
	if len(s.pushes) >= 16 {
		return nil, ErrConflict
	}
	r, w := io.Pipe()
	p := &publicationPipe{r: r, w: w, receiver: true}
	s.pushes[id] = p
	return contextual(ctx, &publicationReader{PipeReader: r, s: s, id: id, p: p}), nil
}
func (s *Service) AttachPublisher(id string) (io.WriteCloser, error) {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	p := s.pushes[id]
	if p == nil {
		if len(s.pushes) >= 16 {
			return nil, ErrConflict
		}
		if s.pushes == nil {
			s.pushes = map[string]*publicationPipe{}
		}
		r, w := io.Pipe()
		p = &publicationPipe{r: r, w: w}
		s.pushes[id] = p
	}
	if p.publisher {
		return nil, ErrConflict
	}
	p.publisher = true
	return &publicationWriter{WriteCloser: p.w, s: s, id: id, p: p}, nil
}

type publicationWriter struct {
	io.WriteCloser
	s    *Service
	id   string
	p    *publicationPipe
	once sync.Once
}

func (w *publicationWriter) Close() error {
	var e error
	w.once.Do(func() {
		e = w.WriteCloser.Close()
		w.s.pushMu.Lock()
		if !w.p.receiver && w.s.pushes[w.id] == w.p {
			delete(w.s.pushes, w.id)
			w.p.r.Close()
		}
		w.s.pushMu.Unlock()
	})
	return e
}
func (s *Service) PublicationWaiting(id string) bool {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()
	return s.pushes[id] != nil && s.pushes[id].receiver
}
func (s *Service) openCluster(ctx context.Context, in Input) (io.ReadCloser, error) {
	client := s.httpClient()
	transport := client.Transport.(*http.Transport)
	transport.TLSClientConfig = clusterTLS(in.Certificate)
	return requestCluster(ctx, in, client, transport.CloseIdleConnections)
}
func clusterTLS(pin string) *tls.Config {
	if pin == "" {
		return nil
	}
	expected, _ := hex.DecodeString(pin)
	return &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return ErrDenied
		}
		sum := sha256.Sum256(cs.PeerCertificates[0].Raw)
		if subtle.ConstantTimeCompare(sum[:], expected) != 1 {
			return ErrDenied
		}
		return nil
	}}
}
func requestCluster(ctx context.Context, in Input, client *http.Client, closeTransport func()) (io.ReadCloser, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if e != nil {
		return nil, ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+in.Token)
	res, e := client.Do(req)
	if e != nil {
		closeTransport()
		return nil, ErrUnavailable
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		closeTransport()
		return nil, ErrUnavailable
	}
	// Closing the dedicated transport bounds connections per configured peer.
	return &clusterBody{ReadCloser: res.Body, closeTransport: closeTransport}, nil
}

type clusterBody struct {
	io.ReadCloser
	closeTransport func()
}

func (r *clusterBody) Close() error { e := r.ReadCloser.Close(); r.closeTransport(); return e }
