package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"kore-edge/internal/operations"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidArgumentsDoNotPrintConfiguration(t *testing.T) {
	for _, args := range [][]string{{}, {"-action", "status", "-config", "SENSITIVE"}, {"-action", "select", "-expected-generation", "bad"}, {"-action", "rollback"}, {"-action", "delete"}, {"-action", "stage", "-revision", "SENSITIVE"}} {
		var out bytes.Buffer
		if e := run(args, &out); e == nil {
			t.Fatal("invalid args accepted")
		}
		if strings.Contains(out.String(), "SENSITIVE") || out.Len() != 0 {
			t.Fatal("sensitive output")
		}
	}
}

func TestOfflineCLIStageAndSelection(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	binary := filepath.Join(dir, "binary")
	configuration := filepath.Join(dir, "config.yaml")
	runtime := filepath.Join(dir, "runtime")
	payload := []byte("synthetic executable")
	sum := sha256.Sum256(payload)
	if e := os.WriteFile(binary, payload, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(configuration, []byte("# SENSITIVE-COMMENT\nversion: 1\nmode: laboratory\nhttp:\n  listen: '127.0.0.1:18080'\n"), 0600); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	if e := run([]string{"-action", "stage", "-directory", runtime, "-binary", binary, "-sha256", hex.EncodeToString(sum[:]), "-config", configuration}, &out); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "SENSITIVE") || strings.Contains(out.String(), "18080") {
		t.Fatal("configuration exposed")
	}
	var manifest operations.Manifest
	if e := json.Unmarshal(out.Bytes(), &manifest); e != nil {
		t.Fatal(e)
	}
	out.Reset()
	if e := run([]string{"-action", "select", "-directory", runtime, "-revision", manifest.ID, "-expected-generation", "0"}, &out); e != nil {
		t.Fatal(e)
	}
	var state operations.State
	if e := json.Unmarshal(out.Bytes(), &state); e != nil || state.Generation != 1 || state.Current != manifest.ID {
		t.Fatal("unexpected selection", e)
	}
	out.Reset()
	if e := run([]string{"-action", "select", "-directory", runtime, "-revision", manifest.ID, "-expected-generation", "0"}, &out); e == nil {
		t.Fatal("stale CLI selection accepted")
	}
	if out.Len() != 0 {
		t.Fatal("failed action returned success output")
	}
}
