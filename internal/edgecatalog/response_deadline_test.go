package edgecatalog

import (
	"context"
	"crypto/x509"
	"errors"
	"kore-edge/internal/wireguard"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMatrixOpeningWithinItsFiveSecondBudgetDoesNotTimeoutAtThreeSeconds(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(3500 * time.Millisecond):
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	client := secureClient(Config{Origin: server.URL, TunnelIP: "127.0.0.1"}, &wireguard.PeerSettings{Address: "127.0.0.2/32"})
	transport := client.Transport.(*http.Transport)
	defer transport.CloseIdleConnections()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport.TLSClientConfig.RootCAs = roots
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("valid matrix startup was interrupted", err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
}

func TestMatrixFailureClassificationPreservesSafeStatusAndTimeout(t *testing.T) {
	for _, code := range []int{429, 503, 504} {
		s, _, _ := fixture(t)
		s.client = func(Config, *wireguard.PeerSettings) *http.Client {
			return &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: code, Body: http.NoBody}, nil
			})}
		}
		_, _, err := s.remote(context.Background(), "/api/kore-edge/catalog")
		var status *RemoteStatusError
		if !errors.As(err, &status) || status.Code != code || !errors.Is(err, ErrUnavailable) {
			t.Fatal("HTTP status lost", err)
		}
	}
	s, _, _ := fixture(t)
	s.client = func(Config, *wireguard.PeerSettings) *http.Client {
		return &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded })}
	}
	_, _, err := s.remote(context.Background(), "/api/kore-edge/catalog")
	if !errors.Is(err, ErrResponseTimeout) || !errors.Is(err, ErrUnavailable) {
		t.Fatal("timeout classification lost", err)
	}
}
