package config

import (
	"kore-edge/internal/provision"
	"testing"
)

func TestProvisionValidation(t *testing.T) {
	var c Config
	c.Version = 1
	c.Mode = "laboratory"
	c.HTTP.Listen = "127.0.0.1:18080"
	c.Provision = provision.Config{Enabled: true, EdgeID: "edge-lab", Directory: t.TempDir()}
	if c.Validate() != nil {
		t.Fatal("valid enrollment config")
	}
	c.Provision.Directory = "relative"
	if c.Validate() == nil {
		t.Fatal("relative runtime directory")
	}
	c.Provision.Directory = t.TempDir()
	c.Provision.EdgeID = "../bad"
	if c.Validate() == nil {
		t.Fatal("invalid edge ID")
	}
}
