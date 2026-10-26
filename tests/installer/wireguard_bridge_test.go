package installer_test

import (
	"os/exec"
	"strings"
	"testing"
)

func TestWireGuardBridgePlanDoesNotRequireRootOrTouchNetwork(t *testing.T) {
	command := exec.Command(bash(t), "../../scripts/install-wireguard-bridge.sh", "--role", "edge", "--user", "synthetic-edge", "--application-state", "/synthetic/private/identity.json", "--binary", "/synthetic/edge-wg-bridge", "--plan")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(output))
	}
	if !strings.Contains(string(output), "SIMULACAO: nenhuma mudanca executada.") || !strings.Contains(string(output), "Rollback") {
		t.Fatal("missing safe plan")
	}
	syntax := exec.Command(bash(t), "-n", "../../scripts/install-wireguard-bridge.sh")
	if output, err := syntax.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
}
