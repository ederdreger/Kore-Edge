package app

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/multicast"
	"kore-edge/internal/stream"
)

func TestUDPPermissionIsDistinctFromHLS(t *testing.T) {
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	store, e := catalog.New("lab", base64.StdEncoding.EncodeToString(pub))
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	m := catalog.Manifest{Version: 1, Revision: 1, PartnerID: "lab", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Channels: []catalog.Channel{{ID: "test", Name: "Synthetic", Source: "synthetic://test", Outputs: []string{"hls"}}}, Grants: []catalog.Grant{{ChannelID: "test", Outputs: []string{"hls"}}}}
	payload, _ := json.Marshal(m)
	body, _ := json.Marshal(catalog.Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, append([]byte(catalog.Domain), payload...)))})
	if store.Apply(body, now) != nil {
		t.Fatal("catalog")
	}
	g := &guardedUDP{store: store, id: "test"}
	if !errors.Is(g.WritePacket(make([]byte, 188)), stream.ErrDenied) {
		t.Fatal("HLS permission used for UDP")
	}
	if multicastOutput(multicast.Config{Mode: "dry_run"}, store) != nil {
		t.Fatal("dry-run created output factory")
	}
}
