package acceptance_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"kore-edge/internal/app"
)

// Helper runs only in a child owned by TestOwnedCoreProcessRestart. It never
// signals existing agents, changes services or reads the user's configuration.
func TestAgentProcessHelper(t *testing.T) {
	if os.Getenv("KORE_EDGE_ACCEPTANCE_HELPER") != "1" {
		return
	}
	listener, e := net.Listen("tcp", os.Getenv("KORE_EDGE_ACCEPTANCE_LISTEN"))
	if e != nil {
		t.Fatal("helper bind failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- app.Serve(ctx, listener) }()
	if e := json.NewEncoder(os.Stdout).Encode(struct {
		Address string `json:"address"`
	}{listener.Addr().String()}); e != nil {
		t.Fatal("helper handshake failed")
	}
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	cancel()
	if e := <-done; e != nil {
		t.Fatal("helper shutdown failed")
	}
}

type child struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	address string
	cancel  context.CancelFunc
	waited  bool
}

func startChild(t *testing.T, address string) *child {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAgentProcessHelper$")
	command.Env = append(os.Environ(), "KORE_EDGE_ACCEPTANCE_HELPER=1", "KORE_EDGE_ACCEPTANCE_LISTEN="+address)
	stdin, e := command.StdinPipe()
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	stdout, e := command.StdoutPipe()
	if e != nil {
		cancel()
		t.Fatal(e)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if e := command.Start(); e != nil {
		cancel()
		t.Fatal("helper could not start")
	}
	c := &child{command: command, stdin: stdin, cancel: cancel}
	t.Cleanup(func() {
		stdin.Close()
		cancel()
		if !c.waited {
			_ = command.Wait()
		}
	})
	var handshake struct {
		Address string `json:"address"`
	}
	if json.NewDecoder(bufio.NewReader(stdout)).Decode(&handshake) != nil {
		t.Fatal("helper did not publish listener")
	}
	host, _, e := net.SplitHostPort(handshake.Address)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatal("helper listener was not loopback")
	}
	c.address = handshake.Address
	return c
}
func (c *child) stop(t *testing.T) {
	t.Helper()
	if _, e := fmt.Fprintln(c.stdin, "stop"); e != nil {
		t.Fatal("helper stop request failed")
	}
	c.stdin.Close()
	e := c.command.Wait()
	c.waited = true
	c.cancel()
	if e != nil {
		t.Fatal("owned core process failed to exit")
	}
}

func TestOwnedCoreProcessRestart(t *testing.T) {
	first := startChild(t, "127.0.0.1:0")
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	check := func(address string) {
		t.Helper()
		for _, path := range []string{"/healthz", "/readyz"} {
			response, e := client.Get("http://" + address + path)
			if e != nil {
				t.Fatal("owned process health failed")
			}
			response.Body.Close()
			if response.StatusCode != 200 {
				t.Fatal("owned process not ready")
			}
		}
	}
	check(first.address)
	firstPID := first.command.Process.Pid
	first.stop(t)
	if connection, e := net.DialTimeout("tcp", first.address, 250*time.Millisecond); e == nil {
		connection.Close()
		t.Fatal("listener survived process shutdown")
	}
	second := startChild(t, first.address)
	check(second.address)
	if second.command.Process.Pid == firstPID {
		t.Fatal("same process reused")
	}
	second.stop(t)
	t.Log("two owned core processes: clean shutdown, listener closed, successful rebind and readiness; no systemd restart")
}
