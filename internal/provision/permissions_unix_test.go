//go:build !windows

package provision

import (
	"os"
	"testing"
)

func TestDirectoryMustBePrivate(t *testing.T) {
	dir := t.TempDir()
	if os.Chmod(dir, 0755) != nil {
		t.Fatal("fixture")
	}
	defer os.Chmod(dir, 0700)
	if _, e := Open(dir); e == nil {
		t.Fatal("public runtime directory accepted")
	}
}
