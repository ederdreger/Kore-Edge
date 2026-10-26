//go:build !windows

package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectPublicSecretPermissions(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "secret")
	if e := os.WriteFile(path, []byte("synthetic"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(path, 1024); e == nil {
		t.Fatal("public secret accepted")
	}
	if e := os.Chmod(path, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(path, 1024); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(dir, 0755); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(path, 1024); e == nil {
		t.Fatal("public directory accepted")
	}
}
