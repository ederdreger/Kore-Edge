// Package installer_test validates the planner, never an authorized application.
package installer_test

import (
	"context"
	"os"
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

func runWizard(t *testing.T, input string, detect bool) (string, error) {
	t.Helper()
	directory := t.TempDir()
	ip := "#!/usr/bin/env bash\nexit 0\n"
	if detect {
		ip = "#!/usr/bin/env bash\nprintf '1: lo inet 127.0.0.1/8 scope host lo\\n2: lo inet6 ::1/128 scope host\\n'\n"
	}
	files := map[string]string{"ip": ip, "ss": "#!/usr/bin/env bash\nexit 0\n"}
	for _, name := range []string{"sudo", "apt-get", "systemctl"} {
		files[name] = "#!/usr/bin/env bash\nprintf invoked >\"$KORE_TEST_MUTATION_MARKER\"\nexit 99\n"
	}
	for name, content := range files {
		if e := os.WriteFile(filepath.Join(directory, name), []byte(content), 0755); e != nil {
			t.Fatal(e)
		}
	}
	marker := filepath.Join(directory, "mutation")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, bash(t), "-c", `directory=$1; if command -v cygpath >/dev/null 2>&1; then directory=$(cygpath -u "$directory"); fi; export PATH="$directory:$PATH"; exec bash ../../scripts/setup.sh`, "wizard-test", directory)
	command.Env = append(os.Environ(), "KORE_TEST_MUTATION_MARKER="+filepath.ToSlash(marker))
	command.Stdin = strings.NewReader(input)
	out, e := command.CombinedOutput()
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("wizard invoked a mutating command")
	}
	return string(out), e
}
func TestWizardGuidedSimulation(t *testing.T) {
	output, e := runWizard(t, "s\n\npainel.example.com\n8443\n\ns\n2\n", true)
	if e != nil {
		t.Fatal(output, e)
	}
	for _, required := range []string{"ETAPA 1/5", "ETAPA 2/5", "ETAPA 3/5", "ETAPA 4/5", "ETAPA 5/5", "Dominio ou IP da VPS", "ffmpeg", "https://painel.example.com:8443/", "127.0.0.1:18080", "Inicio solicitado: SIM", "Simulacao concluida"} {
		if !strings.Contains(output, required) {
			t.Fatalf("missing wizard step %s: %s", required, output)
		}
	}
}
func TestWizardRetriesAndManualAddress(t *testing.T) {
	output, e := runWizard(t, "s\n9\n2\nhttp://bad/\n203.0.113.10\n0\n8443\nadmin\nn\n2\n", true)
	if e != nil || !strings.Contains(output, "Digite 1 ou 2") || !strings.Contains(output, "Endereco invalido") || !strings.Contains(output, "https://203.0.113.10:8443/") {
		t.Fatal(output, e)
	}
	output, e = runWizard(t, "s\n1\n2001:db8::10\n8443\nadmin\nn\n2\n", false)
	if e != nil || !strings.Contains(output, "https://[2001:db8::10]:8443/") || !strings.Contains(output, "Simulacao concluida") {
		t.Fatal(output, e)
	}
}
func TestWizardCancelEOFAndPipeNeverInstall(t *testing.T) {
	for _, input := range []string{"n\n", "s\n", "s\n1\npainel.example.com\n8443\n\nn\n3\n"} {
		output, e := runWizard(t, input, true)
		if e != nil || !strings.Contains(strings.ToLower(output), "cancelad") {
			t.Fatal(output, e)
		}
	}
	output, e := runWizard(t, "s\n1\npainel.example.com\n8443\n\ns\n1\n", true)
	if e == nil || !strings.Contains(output, "terminal interativo") {
		t.Fatal("piped input could apply", output, e)
	}
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
	for _, script := range []string{"install.sh", "setup.sh", "gateway-lib.sh"} {
		command := exec.CommandContext(ctx, bash(t), "-n", "../../scripts/"+script)
		if output, e := command.CombinedOutput(); e != nil {
			t.Fatal("invalid Bash syntax", string(output))
		}
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
