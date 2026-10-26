package wireguard

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type PeerStatus struct {
	Interface      string     `json:"interface"`
	PublicKey      string     `json:"public_key"`
	AllowedIPs     []string   `json:"allowed_ips"`
	LastHandshake  *time.Time `json:"last_handshake"`
	HandshakeState string     `json:"handshake_state"`
}
type Inventory struct {
	Timestamp time.Time    `json:"timestamp"`
	State     string       `json:"state"`
	Source    string       `json:"source"`
	Reason    string       `json:"reason,omitempty"`
	Peers     []PeerStatus `json:"peers"`
}

// Runner is injectable; tests never execute wg or contact peers.
type Runner func(context.Context, ...string) ([]byte, error)
type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 1024*1024 {
		return 0, errors.New("output limit")
	}
	return b.Buffer.Write(p)
}
func run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "wg", args...)
	var out limitedBuffer
	cmd.Stdout = &out // stderr discarded: never propagate command output as errors.
	cmd.WaitDelay = 100 * time.Millisecond
	if err := cmd.Run(); err != nil {
		return nil, errors.New("WireGuard query unavailable")
	}
	return out.Bytes(), nil
}
func Collect(ctx context.Context) Inventory {
	now := time.Now().UTC()
	if runtime.GOOS != "linux" {
		return Inventory{Timestamp: now, State: "unavailable", Source: "wg show", Reason: "unsupported operating system"}
	}
	return ReadInventory(ctx, run, now)
}

func ReadInventory(ctx context.Context, runner Runner, now time.Time) Inventory {
	result := Inventory{Timestamp: now, State: "unavailable", Source: "wg show all latest-handshakes + allowed-ips", Reason: "query unavailable or invalid"}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	handshakes, e := runner(ctx, "show", "all", "latest-handshakes")
	if e != nil || len(handshakes) > 1024*1024 {
		return result
	}
	allowed, e := runner(ctx, "show", "all", "allowed-ips")
	if e != nil || len(allowed) > 1024*1024 {
		return result
	}
	peers := []PeerStatus{}
	indices := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(handshakes)), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 || !namePattern.MatchString(f[0]) || !validKey(f[1]) {
			return result
		}
		id := f[0] + ":" + f[1]
		if _, ok := indices[id]; ok {
			return result
		}
		n, e := strconv.ParseInt(f[2], 10, 64)
		if e != nil || n < 0 || n > now.Unix() {
			return result
		}
		p := PeerStatus{Interface: f[0], PublicKey: f[1], AllowedIPs: []string{}, HandshakeState: "never"}
		if n > 0 {
			ts := time.Unix(n, 0).UTC()
			p.LastHandshake = &ts
			p.HandshakeState = "observed"
		}
		indices[id] = len(peers)
		peers = append(peers, p)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(allowed)), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 3 {
			return result
		}
		id := f[0] + ":" + f[1]
		i, ok := indices[id]
		if !ok || seen[id] {
			return result
		}
		seen[id] = true
		if f[2] == "(none)" {
			if len(f) != 3 {
				return result
			}
			continue
		}
		for _, raw := range f[2:] {
			for _, cidr := range strings.Split(raw, ",") {
				if cidr == "" {
					continue
				}
				if _, e := netip.ParsePrefix(cidr); e != nil {
					return result
				}
				peers[i].AllowedIPs = append(peers[i].AllowedIPs, cidr)
			}
		}
	}
	if len(seen) != len(peers) {
		return result
	} // two queries can race; do not invent a complete snapshot.
	result.State = "real"
	result.Reason = ""
	result.Peers = peers
	return result
}
