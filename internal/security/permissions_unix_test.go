//go:build !windows

package security

import (
	"os"
	"os/exec"
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
	// Creation mode is filtered by the caller's umask (077 in the installer).
	// Explicitly establish the insecure fixture before testing its rejection.
	if e := os.Chmod(path, 0644); e != nil {
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

func TestSecretPermissionsUnderInstallerUmask(t *testing.T) {
	executable, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	command := exec.Command("bash", "-c", `umask 077; exec "$1" -test.run=^TestRejectPublicSecretPermissions$`, "permissions-fixture", executable)
	if output, e := command.CombinedOutput(); e != nil {
		t.Fatalf("permissions test under installer umask failed: %v\n%s", e, output)
	}
}
