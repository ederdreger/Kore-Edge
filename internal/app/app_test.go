package app

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"kore-edge/internal/config"
)

func TestLifecycle(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln) }()
	client := &http.Client{Timeout: time.Second}
	for _, path := range []string{"/healthz", "/readyz", "/api/v1/monitoring", "/metrics"} {
		resp, err := client.Get("http://" + ln.Addr().String() + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s: %d", path, resp.StatusCode)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("shutdown timeout")
	}
	if conn, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		conn.Close()
		t.Fatal("listener still open")
	}
}

func TestInvalidConfig(t *testing.T) {
	if Run(context.Background(), config.Config{}) == nil {
		t.Fatal("invalid config accepted")
	}
}
