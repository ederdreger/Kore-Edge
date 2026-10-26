package config

import (
	"kore-edge/internal/accounts"
	"kore-edge/internal/multicast"
	"path/filepath"
	"testing"
)

func TestCleanProductionRequiresAccountsAndRejectsSyntheticModules(t *testing.T) {
	var c Config
	c.Version = 1
	c.Mode = "production"
	c.HTTP.Listen = "127.0.0.1:18080"
	c.Accounts = accounts.Config{Enabled: true, Directory: filepath.Join(t.TempDir(), "users"), PublicOrigin: "https://edge.example"}
	if c.Validate() != nil {
		t.Fatal("clean production rejected")
	}
	for _, mutate := range []func(*Config){func(x *Config) { x.Accounts.Enabled = false }, func(x *Config) { x.Accounts.PublicOrigin = "http://edge.example" }, func(x *Config) { x.Stream.Enabled = true }, func(x *Config) { x.HLS.Enabled = true }, func(x *Config) { x.Integration.Enabled = true }, func(x *Config) { x.Provision.Enabled = true }, func(x *Config) { x.Multicast.Outputs = []multicast.Target{{}} }} {
		next := c
		mutate(&next)
		if next.Validate() == nil {
			t.Fatal("unsafe production accepted")
		}
	}
}
