package config

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"kore-edge/internal/integration"
	"path/filepath"
	"strings"
	"testing"
)

func TestIntegrationRequiresMockAndCatalogTrust(t *testing.T) {
	for _, extra := range []string{"integration:\n  enabled: true\n  mode: remote\n", "integration:\n  enabled: true\n  mode: mock\n  edge_id: edge-lab\n  mock_catalog: /private/catalog.json\n  interval_seconds: 1\n", "integration:\n  endpoint: https://production.invalid\n"} {
		if _, e := Parse(strings.NewReader("version: 1\nmode: laboratory\nhttp:\n  listen: '127.0.0.1:18080'\n" + extra)); e == nil {
			t.Fatal("untrusted integration config accepted")
		}
	}
}

func TestTrustedMockConfigurationAndIdentityMatch(t *testing.T) {
	public, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	c := Config{Version: 1, Mode: "laboratory"}
	c.HTTP.Listen = "127.0.0.1:18080"
	c.Catalog.PartnerID = "partner-lab"
	c.Catalog.PublicKey = base64.StdEncoding.EncodeToString(public)
	c.Integration = integration.Config{Enabled: true, Mode: "mock", EdgeID: "edge-lab", MockCatalog: filepath.Join(t.TempDir(), "mock.json"), IntervalSeconds: 30}
	if e := c.Validate(); e != nil {
		t.Fatal("valid mock rejected", e)
	}
	c.Provision.Enabled = true
	c.Provision.Directory = t.TempDir()
	c.Provision.EdgeID = "other-edge"
	if c.Validate() == nil {
		t.Fatal("mismatched identity accepted")
	}
	c.Provision.EdgeID = "edge-lab"
	if e := c.Validate(); e != nil {
		t.Fatal("matching identity rejected", e)
	}
}
