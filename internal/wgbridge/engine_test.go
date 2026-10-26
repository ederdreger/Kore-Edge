package wgbridge

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"kore-edge/internal/wireguard"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeKernel struct {
	mu         sync.Mutex
	exists     bool
	up         bool
	address    string
	public     string
	port       int
	peers      map[string][]string
	routes     map[string]string
	calls      []string
	fail       string
	occupied   bool
	wrongRoute bool
	handshake  bool
}

func fake(t *testing.T) (*Engine, *fakeKernel, *wireguard.ManagedConfig) {
	t.Helper()
	key, _ := ecdh.X25519().GenerateKey(rand.Reader)
	raw := key.Bytes()
	raw[0] &= 248
	raw[31] &= 127
	raw[31] |= 64
	key, _ = ecdh.X25519().NewPrivateKey(raw)
	remote, _ := ecdh.X25519().GenerateKey(rand.Reader)
	config := &wireguard.ManagedConfig{Interface: "wg-test", Address: "10.254.110.2/32", ListenPort: 1605, PublicKey: base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), Secret: base64.StdEncoding.EncodeToString(raw), Peers: []wireguard.Peer{{PublicKey: base64.StdEncoding.EncodeToString(remote.PublicKey().Bytes()), Endpoint: "203.0.113.7:1605", AllowedIPs: []string{"10.254.110.1/32"}, Keepalive: 25}}}
	kernel := &fakeKernel{routes: map[string]string{"192.168.40.0/24": "eth0"}, peers: map[string][]string{}}
	engine, err := New(filepath.Join(t.TempDir(), "bridge"), func() (wireguard.ManagedConfig, error) {
		c := *config
		c.Peers = append([]wireguard.Peer(nil), config.Peers...)
		return c, nil
	}, kernel.run)
	if err != nil {
		t.Fatal(err)
	}
	return engine, kernel, config
}
func (k *fakeKernel) run(_ context.Context, program string, args ...string) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	call := program + " " + strings.Join(args, " ")
	k.calls = append(k.calls, call)
	if k.fail != "" && strings.Contains(call, k.fail) {
		k.fail = ""
		return nil, ErrUnavailable
	}
	encode := func(v any) ([]byte, error) { b, e := json.Marshal(v); return b, e }
	if program == "ss" {
		if k.occupied || k.exists {
			return []byte("UNCONN 0 0 *:1605 *:*"), nil
		}
		return nil, nil
	}
	if program == "wg" {
		if args[0] == "syncconf" {
			if !k.exists {
				return nil, ErrUnavailable
			}
			b, e := os.ReadFile(args[2])
			if e != nil {
				return nil, e
			}
			k.peers = map[string][]string{}
			peer := ""
			for _, line := range strings.Split(string(b), "\n") {
				parts := strings.SplitN(line, " = ", 2)
				if len(parts) != 2 {
					continue
				}
				switch parts[0] {
				case "PrivateKey":
					raw, _ := base64.StdEncoding.DecodeString(parts[1])
					key, e := ecdh.X25519().NewPrivateKey(raw)
					if e != nil {
						return nil, e
					}
					k.public = base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
				case "ListenPort":
					k.port, _ = strconv.Atoi(parts[1])
				case "PublicKey":
					peer = parts[1]
				case "AllowedIPs":
					k.peers[peer] = strings.Split(strings.ReplaceAll(parts[1], " ", ""), ",")
				}
			}
			return nil, nil
		}
		if !k.exists {
			return nil, ErrUnavailable
		}
		switch args[2] {
		case "public-key":
			return []byte(k.public), nil
		case "listen-port":
			return []byte(strconv.Itoa(k.port)), nil
		case "allowed-ips":
			keys := []string{}
			for key := range k.peers {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			out := ""
			for _, key := range keys {
				out += key + "\t" + strings.Join(k.peers[key], ",") + "\n"
			}
			return []byte(out), nil
		case "latest-handshakes":
			out := ""
			for key := range k.peers {
				stamp := "0"
				if k.handshake {
					stamp = strconv.FormatInt(time.Now().Unix(), 10)
				}
				out += key + "\t" + stamp + "\n"
			}
			return []byte(out), nil
		}
		return nil, ErrUnavailable
	}
	if program != "ip" {
		return nil, ErrUnavailable
	}
	if strings.HasPrefix(strings.Join(args, " "), "-j address show") {
		links := []any{map[string]any{"ifname": "eth0", "addr_info": []any{map[string]any{"local": "192.168.40.2", "prefixlen": 24}}}}
		if k.exists {
			addresses := []any{}
			if k.address != "" {
				parts := strings.Split(k.address, "/")
				bits, _ := strconv.Atoi(parts[1])
				addresses = append(addresses, map[string]any{"local": parts[0], "prefixlen": bits})
			}
			links = append(links, map[string]any{"ifname": "wg-test", "addr_info": addresses})
		}
		return encode(links)
	}
	if len(args) > 0 && args[0] == "-6" {
		if len(args) > 1 && args[1] == "-j" {
			return []byte("[]"), nil
		}
		args = args[1:]
	}
	if len(args) > 1 && args[0] == "-j" && args[1] == "rule" {
		return []byte(`[{"priority":0,"table":"local"},{"priority":32766,"table":"main"}]`), nil
	}
	if len(args) > 2 && args[0] == "-j" && args[1] == "route" && args[2] == "show" {
		out := []any{map[string]any{"dst": "default", "dev": "eth0", "gateway": "192.168.40.1"}}
		for raw, dev := range k.routes {
			out = append(out, map[string]any{"dst": raw, "dev": dev, "table": "main"})
		}
		return encode(out)
	}
	if len(args) > 2 && args[0] == "-j" && args[1] == "route" && args[2] == "get" {
		dev := k.routes[args[3]+"/32"]
		if k.wrongRoute {
			dev = "eth0"
		}
		return encode([]any{map[string]any{"dev": dev}})
	}
	if len(args) > 2 && args[0] == "-j" && args[1] == "link" {
		if !k.exists {
			return nil, ErrUnavailable
		}
		flags := []string{}
		if k.up {
			flags = append(flags, "UP")
		}
		return encode([]any{map[string]any{"flags": flags}})
	}
	if args[0] == "link" {
		switch args[1] {
		case "add":
			if k.exists {
				return nil, ErrConflict
			}
			k.exists = true
		case "set":
			k.up = true
		case "delete":
			if !k.exists {
				return nil, ErrConflict
			}
			k.exists = false
			k.address = ""
			k.up = false
			for route, dev := range k.routes {
				if dev == "wg-test" {
					delete(k.routes, route)
				}
			}
		}
		return nil, nil
	}
	if args[0] == "address" {
		k.address = args[2]
		return nil, nil
	}
	if args[0] == "route" {
		switch args[1] {
		case "add":
			if _, ok := k.routes[args[2]]; ok {
				return nil, ErrConflict
			}
			k.routes[args[2]] = args[4]
		case "delete":
			if k.routes[args[2]] != args[4] {
				return nil, ErrConflict
			}
			delete(k.routes, args[2])
		}
		return nil, nil
	}
	return nil, ErrUnavailable
}
func wait(t *testing.T, e *Engine) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if e.Wait(ctx) != nil {
		t.Fatal("worker did not finish")
	}
}
func TestReviewedApplyStatusAndRollback(t *testing.T) {
	e, k, c := fake(t)
	p, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), c.Secret) || strings.Contains(string(b), "Secret") {
		t.Fatal("plan leaked key")
	}
	if _, err = e.Apply(p.Digest); err != nil {
		t.Fatal(err)
	}
	wait(t, e)
	s := e.Status(context.Background())
	if !s.Applied || s.HandshakeRecent {
		t.Fatal("invented connectivity", s)
	}
	k.mu.Lock()
	k.handshake = true
	k.mu.Unlock()
	if !e.Status(context.Background()).HandshakeRecent {
		t.Fatal("fresh handshake missing")
	}
	p, err = e.Plan(context.Background())
	if err != nil || !p.Applied {
		t.Fatal("not idempotent", err)
	}
	journal, _ := os.ReadFile(filepath.Join(e.directory, "journal.json"))
	if strings.Contains(string(journal), c.Secret) {
		t.Fatal("journal leaked key")
	}
	if _, err = e.Rollback(); err != nil {
		t.Fatal(err)
	}
	wait(t, e)
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.exists || k.routes["192.168.40.0/24"] != "eth0" {
		t.Fatal("rollback touched unrelated state")
	}
	for _, command := range k.calls {
		if strings.Contains(command, c.Secret) {
			t.Fatal("key in argv")
		}
		if strings.Contains(command, "default") || strings.Contains(command, "iptables") {
			t.Fatal("unreviewed network action")
		}
	}
}
func TestConflictsFailBeforeMutation(t *testing.T) {
	for _, kind := range []string{"unowned", "port", "route", "endpoint"} {
		t.Run(kind, func(t *testing.T) {
			e, k, c := fake(t)
			switch kind {
			case "unowned":
				k.exists = true
				k.public = "other"
			case "port":
				k.occupied = true
			case "route":
				c.Peers[0].AllowedIPs = []string{"192.168.40.1/32"}
			case "endpoint":
				c.Peers[0].Endpoint = "10.254.110.1:1605"
			}
			if _, err := e.Plan(context.Background()); err == nil {
				t.Fatal("conflict accepted")
			}
			for _, call := range k.calls {
				if strings.Contains(call, "link add") || strings.Contains(call, "syncconf") {
					t.Fatal("mutation during plan")
				}
			}
		})
	}
}
func TestChangedPlanAndFailedApplyRestoreOwnedResources(t *testing.T) {
	for _, failure := range []string{"changed", "syncconf", "address add", "route add", "policy"} {
		t.Run(failure, func(t *testing.T) {
			e, k, _ := fake(t)
			p, err := e.Plan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "changed":
				p.Digest = strings.Repeat("0", 64)
			case "policy":
				k.wrongRoute = true
			default:
				k.fail = failure
			}
			if _, err = e.Apply(p.Digest); err != nil {
				t.Fatal(err)
			}
			wait(t, e)
			if e.Status(context.Background()).Stage != "failed" {
				t.Fatal("failure hidden")
			}
			k.mu.Lock()
			defer k.mu.Unlock()
			if k.exists || len(k.routes) != 1 || k.routes["192.168.40.0/24"] != "eth0" {
				t.Fatal("partial state left behind")
			}
		})
	}
}
func TestUpdatesPreserveExistingInterfaceAndCanUndo(t *testing.T) {
	e, k, c := fake(t)
	p, _ := e.Plan(context.Background())
	e.Apply(p.Digest)
	wait(t, e)
	remote, _ := ecdh.X25519().GenerateKey(rand.Reader)
	c.Peers = append(c.Peers, wireguard.Peer{PublicKey: base64.StdEncoding.EncodeToString(remote.PublicKey().Bytes()), Endpoint: "203.0.113.8:1605", AllowedIPs: []string{"10.254.110.3/32"}})
	k.mu.Lock()
	start := len(k.calls)
	k.mu.Unlock()
	p, err := e.Plan(context.Background())
	if err != nil || !p.ExistingOwned {
		t.Fatal(err)
	}
	e.Apply(p.Digest)
	wait(t, e)
	if !e.Status(context.Background()).Applied {
		t.Fatal("update failed")
	}
	k.mu.Lock()
	for _, call := range k.calls[start:] {
		if strings.Contains(call, "link add") || strings.Contains(call, "link delete") {
			t.Fatal("recreated healthy interface")
		}
	}
	k.mu.Unlock()
	e.Rollback()
	wait(t, e)
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.exists || len(k.peers) != 1 || k.routes["10.254.110.1/32"] != "wg-test" || k.routes["10.254.110.3/32"] != "" {
		t.Fatal("previous peer state not restored")
	}
}
