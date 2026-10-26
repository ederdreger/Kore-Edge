package app

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/integration"
	"kore-edge/internal/security"
)

func TestMockSynchronizerLifecycleAndStatus(t *testing.T) {
	public, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	store, e := catalog.New("partner-lab", base64.StdEncoding.EncodeToString(public))
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private dir")
	}
	path := filepath.Join(dir, "mock.json")
	now := time.Now().UTC()
	payload, e := json.Marshal(catalog.Manifest{Version: 1, Revision: 1, PartnerID: "partner-lab", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Channels: []catalog.Channel{}, Grants: []catalog.Grant{}})
	if e != nil {
		t.Fatal(e)
	}
	body, e := json.Marshal(catalog.Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, append([]byte(catalog.Domain), payload...)))})
	if e != nil {
		t.Fatal(e)
	}
	if os.WriteFile(path, body, 0600) != nil {
		t.Fatal("fixture")
	}
	synchronizer, e := integration.NewMock(integration.Config{Enabled: true, Mode: "mock", EdgeID: "edge-lab", MockCatalog: path, IntervalSeconds: 1}, store)
	if e != nil {
		t.Fatal(e)
	}
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveIntegrated(ctx, ln, store, nil, nil, nil, "", security.Local(), synchronizer) }()
	client := &http.Client{Timeout: time.Second}
	deadline := time.After(3 * time.Second)
	for {
		response, e := client.Get("http://" + ln.Addr().String() + "/api/v1/integration")
		if e != nil {
			t.Fatal(e)
		}
		var status integration.Status
		e = json.NewDecoder(response.Body).Decode(&status)
		response.Body.Close()
		if e != nil {
			t.Fatal(e)
		}
		if status.CatalogAvailable && status.Revision == 1 && status.Mode == "mock" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("mock never synchronized")
		case <-time.After(5 * time.Millisecond):
		}
	}
	response, e := client.Get("http://" + ln.Addr().String() + "/readyz")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("sync changed readiness")
	}
	cancel()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("shutdown blocked by synchronizer")
	}
	if synchronizer.Snapshot(time.Now().UTC()).State != "stopped" {
		t.Fatal("worker survived shutdown")
	}
}
