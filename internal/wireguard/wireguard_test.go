package wireguard

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func example(t *testing.T) Request {
	t.Helper()
	b, e := os.ReadFile("../../configs/wireguard-plan.example.json")
	if e != nil {
		t.Fatal(e)
	}
	var r Request
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	return r
}

func TestDryRunAndRollback(t *testing.T) {
	r := example(t)
	before := r.Desired
	before.ListenPort = 51822
	r.Before = &before
	plan, e := DryRun(r)
	if e != nil {
		t.Fatal(e)
	}
	repeat, e := DryRun(r)
	if e != nil || !reflect.DeepEqual(plan, repeat) {
		t.Fatal("not deterministic")
	}
	expected, _ := Render(before)
	if plan.Applied || plan.Mode != "dry_run" || plan.RollbackTemplate != expected || !strings.Contains(plan.Template, "Table = off") || strings.Contains(plan.Template, "PrivateKey =") {
		t.Fatal("unsafe plan")
	}
	reverse, e := DryRun(Request{Before: &r.Desired, Desired: before})
	if e != nil || reverse.Template != expected || reverse.RollbackTemplate != plan.Template {
		t.Fatal("rollback roundtrip")
	}
	empty, e := DryRun(example(t))
	if e != nil || empty.RollbackTemplate != "" || !strings.Contains(empty.RollbackAction, "newly approved") {
		t.Fatal("creation rollback")
	}
}

func TestRejectedPlans(t *testing.T) {
	for _, cidr := range []string{"0.0.0.0/0", "::/0", "0.0.0.0/1", "128.0.0.0/1", "239.1.1.0/24", "8.8.8.8/32", "10.250.0.1/24", "10.250.0.2/32", "bad"} {
		r := example(t)
		r.Desired.Peers[0].AllowedIPs = []string{cidr}
		if _, e := DryRun(r); e == nil {
			t.Fatalf("accepted %s", cidr)
		}
	}
	for _, mutate := range []func(*Config){
		func(c *Config) { c.Interface = "wg0\nPostUp=x" }, func(c *Config) { c.ListenPort = 0 },
		func(c *Config) { c.Address = "8.8.8.8/32" }, func(c *Config) { c.Peers[0].PublicKey = "secret" },
		func(c *Config) { c.Peers[0].Endpoint = "example.com:123" }, func(c *Config) { c.Peers[0].Endpoint = "8.8.8.8:123" },
		func(c *Config) { c.Peers[0].AllowedIPs = append(c.Peers[0].AllowedIPs, "10.250.0.1/32") },
		func(c *Config) { c.Peers = append(c.Peers, c.Peers[0]) }, func(c *Config) { c.Peers[0].Keepalive = -1 },
	} {
		r := example(t)
		mutate(&r.Desired)
		if _, e := DryRun(r); e == nil {
			t.Fatal("accepted invalid plan")
		}
	}
	r := example(t)
	b := r.Desired
	b.Interface = "other"
	r.Before = &b
	if _, e := DryRun(r); e == nil {
		t.Fatal("rollback mismatch")
	}
}

func TestInventory(t *testing.T) {
	key := example(t).Desired.Peers[0].PublicKey
	now := time.Unix(1000, 0).UTC()
	for _, epoch := range []string{"0", "999"} {
		calls := 0
		runner := func(ctx context.Context, args ...string) ([]byte, error) {
			calls++
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("missing timeout")
			}
			if strings.Join(args, " ") == "show all latest-handshakes" {
				return []byte("wg-lab\t" + key + "\t" + epoch + "\n"), nil
			}
			if strings.Join(args, " ") != "show all allowed-ips" {
				t.Fatal("unexpected command")
			}
			return []byte("wg-lab\t" + key + "\t10.250.0.1/32\n"), nil
		}
		s := ReadInventory(context.Background(), runner, now)
		if s.State != "real" || len(s.Peers) != 1 || calls != 2 || !s.Timestamp.Equal(now) {
			t.Fatalf("bad inventory %+v", s)
		}
		if (s.Peers[0].LastHandshake == nil) != (epoch == "0") {
			t.Fatal("handshake")
		}
	}
	failed := ReadInventory(context.Background(), func(context.Context, ...string) ([]byte, error) { return nil, errors.New("secret output") }, now)
	if failed.State != "unavailable" || strings.Contains(failed.Reason, "secret") {
		t.Fatal("leaked error")
	}
	malformed := ReadInventory(context.Background(), func(context.Context, ...string) ([]byte, error) { return []byte("bad"), nil }, now)
	if malformed.State != "unavailable" {
		t.Fatal("malformed accepted")
	}
	empty := ReadInventory(context.Background(), func(context.Context, ...string) ([]byte, error) { return nil, nil }, now)
	if empty.State != "real" || len(empty.Peers) != 0 {
		t.Fatal("empty host")
	}
}
