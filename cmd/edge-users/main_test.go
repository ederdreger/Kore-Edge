package main

import (
	"kore-edge/internal/config"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareCleanConfig(t *testing.T) {
	d := t.TempDir()
	old := filepath.Join(d, "old.yaml")
	next := filepath.Join(d, "new.yaml")
	os.WriteFile(old, []byte("version: 1\nmode: laboratory\nhttp:\n  listen: 127.0.0.1:18080\n"), 0600)
	args := []string{"--action", "prepare-config", "--config", old, "--directory", filepath.Join(d, "users"), "--origin", "https://edge.example", "--output", next}
	if e := run(args); e != nil {
		t.Fatal(e)
	}
	c, e := config.Load(next)
	if e != nil || c.Mode != "production" || !c.Accounts.Enabled || c.Stream.Enabled || c.Integration.Enabled {
		t.Fatal("unclean config", e)
	}
	if run(args) == nil {
		t.Fatal("overwrote config")
	}
	b, _ := os.ReadFile(next)
	if strings.Contains(string(b), "password") {
		t.Fatal("credential written")
	}
}
func TestRejectInvalidActions(t *testing.T) {
	for _, args := range [][]string{{}, {"--action", "reset-htpasswd"}, {"--action", "unknown"}, {"--action", "prepare-config", "--config", "missing"}} {
		if run(args) == nil {
			t.Fatal("invalid command accepted")
		}
	}
}
