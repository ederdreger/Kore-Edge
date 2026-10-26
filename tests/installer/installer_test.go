// Package installer_test validates the planner, never an authorized application.
package installer_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func bash(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return `C:\Program Files\Git\bin\bash.exe`
	}
	path, e := exec.LookPath("bash")
	if e != nil {
		t.Fatal("bash required for installer checks")
	}
	return path
}
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	script := filepath.ToSlash(filepath.Join("..", "..", "scripts", "install.sh"))
	command := exec.CommandContext(ctx, bash(t), append([]string{script}, args...)...)
	output, e := command.CombinedOutput()
	return string(output), e
}
func TestInstallerSyntax(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, bash(t), "-n", "../../scripts/install.sh")
	if output, e := command.CombinedOutput(); e != nil {
		t.Fatal("invalid Bash syntax", string(output))
	}
}
func TestPlanIncludesPackagesAndNeverApplies(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:18080", "[::1]:18080"} {
		output, e := run(t, "--plan", "--listen", listen, "--profile", "media")
		if e != nil {
			t.Fatal(output)
		}
		for _, required := range []string{"ffmpeg", "wireguard-tools", "golang-go", "build-essential", "SIMULACAO", "NO-GO", listen} {
			if !strings.Contains(output, required) {
				t.Fatalf("missing plan detail %s", required)
			}
		}
	}
	output, e := run(t, "--listen", "127.0.0.1:18080", "--profile", "core")
	if e != nil || strings.Contains(output, "wireguard-tools") || !strings.Contains(output, "SIMULACAO") {
		t.Fatal("core plan incorrect", output)
	}
}
func TestInvalidArgumentsFailBeforeAnyOperation(t *testing.T) {
	for _, args := range [][]string{{}, {"--listen"}, {"--listen", "0.0.0.0:8080"}, {"--listen", "127.0.0.1:0"}, {"--listen", "127.0.0.1:65536"}, {"--listen", "127.0.999.1:8080"}, {"--listen", "127.00.0.1:8080"}, {"--listen", "localhost:8080"}, {"--listen", "127.0.0.1:08080"}, {"--listen", "127.0.0.1:8080", "--profile", "invalid"}, {"--listen", "127.0.0.1:8080", "--apply"}, {"--unknown"}} {
		output, e := run(t, args...)
		if e == nil || !strings.Contains(output, "ERRO:") {
			t.Fatal("invalid invocation accepted", args, output)
		}
	}
}
