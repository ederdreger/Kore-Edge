package wireguard

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type pairingTransport func(*http.Request) (*http.Response, error)

func (f pairingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPairingAuthenticatedReplyAndIdempotence(t *testing.T) {
	s, err := OpenCredentials(filepath.Join(t.TempDir(), "identity"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial, err := s.EnsureIdentity()
	if err != nil {
		t.Fatal(err)
	}
	matrix, _ := ecdh.X25519().GenerateKey(rand.Reader)
	inv := Invitation{Version: 1, APIOrigin: "https://matrix.example", Token: strings.Repeat("a", 43), EdgePublicKey: initial.PublicKey, MatrixPublicKey: base64.StdEncoding.EncodeToString(matrix.PublicKey().Bytes()), Address: "10.77.0.2/32", Expires: time.Now().Add(10 * time.Minute).UnixMilli()}
	raw, _ := json.Marshal(inv)
	pub, _ := base64.StdEncoding.DecodeString(initial.PublicKey)
	edge, _ := ecdh.X25519().NewPublicKey(pub)
	shared, _ := matrix.ECDH(edge)
	tamper := false
	client := &http.Client{Transport: pairingTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != inv.APIOrigin+"/api/wireguard/connect/claim" {
			t.Fatal("unexpected destination")
		}
		var request map[string]string
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("request")
		}
		nonce := sign(shared, "kore-edge/connect/nonce/v1\n"+inv.Token+"\n"+inv.EdgePublicKey)
		if request["nonce"] != nonce || request["proof"] != sign(shared, "kore-edge/connect/v1\n"+inv.Token+"\n"+inv.EdgePublicKey+"\n"+nonce) {
			t.Fatal("invalid ownership proof")
		}
		payload, _ := json.Marshal(map[string]any{"version": 1, "edge_public_key": inv.EdgePublicKey, "matrix_public_key": inv.MatrixPublicKey, "address": inv.Address, "edge_endpoint": "", "kore_play_endpoint": "matrix.example:51820", "allowed_ips": []string{"10.77.0.1/32"}})
		encoded := base64.StdEncoding.EncodeToString(payload)
		signature := sign(shared, "kore-edge/connect/reply/v1\n"+nonce+"\n"+encoded)
		if tamper {
			signature = strings.Repeat("0", 64)
		}
		body, _ := json.Marshal(map[string]string{"payload": encoded, "proof": signature})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: make(http.Header)}, nil
	})}
	local := LocalConnection{Interface: "wg-edge", ListenPort: 51821, Keepalive: 25}
	configured, err := s.claimConnection(context.Background(), string(raw), local, initial.Revision, client)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Applied || configured.Settings.Address != inv.Address {
		t.Fatal("must prepare public settings only")
	}
	again, err := s.claimConnection(context.Background(), string(raw), local, configured.Revision, client)
	if err != nil || again.Revision != configured.Revision {
		t.Fatal("retry changed revision", err)
	}
	tamper = true
	if _, err = s.claimConnection(context.Background(), string(raw), local, configured.Revision, client); err == nil {
		t.Fatal("forged reply accepted")
	}
	unchanged, _ := s.Snapshot()
	if unchanged.Revision != configured.Revision {
		t.Fatal("tamper changed state")
	}
	for _, origin := range []string{"http://matrix.example", "https://matrix.example/path", "https://user:password@matrix.example", "https://127.0.0.1", "https://matrix.example?token=x"} {
		inv.APIOrigin = origin
		bad, _ := json.Marshal(inv)
		if _, err := ParseInvitation(string(bad)); err == nil {
			t.Fatal("unsafe origin accepted", origin)
		}
	}
	inv.APIOrigin = "https://matrix.example"
	inv.Expires = time.Now().Add(-time.Second).UnixMilli()
	expired, _ := json.Marshal(inv)
	if _, err := ParseInvitation(string(expired)); err == nil {
		t.Fatal("expired invitation accepted")
	}
}
