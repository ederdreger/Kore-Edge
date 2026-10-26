package provision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The helper receives test-only credentials in memory/environment, never argv,
// stdout, stderr or a versioned fixture. Only fixed exit codes leave the child.
func TestRegistryProcess(t *testing.T) {
	if os.Getenv("KORE_EDGE_TEST_ENROLL_CHILD") != "1" {
		return
	}
	r, e := Open(os.Getenv("KORE_EDGE_TEST_ENROLL_DIR"))
	if e != nil {
		os.Exit(5)
	}
	body, e := base64.StdEncoding.DecodeString(os.Getenv("KORE_EDGE_TEST_ENROLL_PROOF"))
	if e != nil {
		os.Exit(5)
	}
	var request Request
	if json.Unmarshal(body, &request) != nil {
		os.Exit(5)
	}
	_, e = r.Register(context.Background(), "edge-lab", os.Getenv("KORE_EDGE_TEST_ENROLL_BEARER"), request, time.Now().UTC())
	if e == nil {
		os.Exit(0)
	}
	if errors.Is(e, ErrRejected) {
		os.Exit(3)
	}
	os.Exit(5)
}
func TestOneUseAcrossOperatingSystemProcesses(t *testing.T) {
	r, a, _ := setup(t)
	credential, e := r.Issue(context.Background(), a, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	request := proof(t, a.EdgeID)
	payload, _ := json.Marshal(request)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var accepted, rejected, failed atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRegistryProcess$")
			cmd.Env = append(os.Environ(), "KORE_EDGE_TEST_ENROLL_CHILD=1", "KORE_EDGE_TEST_ENROLL_DIR="+r.directory, "KORE_EDGE_TEST_ENROLL_BEARER="+credential.Token, "KORE_EDGE_TEST_ENROLL_PROOF="+base64.StdEncoding.EncodeToString(payload))
			e := cmd.Run()
			if e == nil {
				accepted.Add(1)
				return
			}
			var exit *exec.ExitError
			if errors.As(e, &exit) && exit.ExitCode() == 3 {
				rejected.Add(1)
			} else {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 || rejected.Load() != 3 || failed.Load() != 0 {
		t.Fatal("one-use transaction failed across processes")
	}
}
