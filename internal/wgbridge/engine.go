// Package wgbridge implements the explicitly installed local network delegate.
// It accepts no browser-supplied commands, configuration paths or private keys.
package wgbridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"kore-edge/internal/security"
	"kore-edge/internal/wireguard"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("bridge_unavailable")
var ErrConflict = errors.New("network_conflict_or_changed_plan")

type Runner func(context.Context, string, ...string) ([]byte, error)
type Plan struct {
	Digest        string                  `json:"digest"`
	DesiredDigest string                  `json:"desired_digest"`
	Config        wireguard.ManagedConfig `json:"config"`
	RoutesAdded   []string                `json:"routes_added"`
	RoutesRemoved []string                `json:"routes_removed"`
	ExistingOwned bool                    `json:"existing_owned"`
	Applied       bool                    `json:"applied"`
}
type Status struct {
	Stage                string       `json:"stage"`
	Job                  string       `json:"job"`
	Error                string       `json:"error"`
	Applied              bool         `json:"applied"`
	DesiredDigest        string       `json:"desired_digest"`
	HandshakeRecent      bool         `json:"handshake_recent"`
	CanRollback          bool         `json:"can_rollback"`
	CheckedAt            string       `json:"checked_at"`
	ObservationAvailable bool         `json:"observation_available"`
	Peers                []PeerStatus `json:"peers"`
}

type PeerStatus struct {
	PublicKey     string `json:"public_key"`
	LastHandshake int64  `json:"last_handshake"`
	Online        bool   `json:"online"`
}

// Only the configured keys count as evidence. Missing, duplicate or malformed
// observations are unavailable, never fabricated offline/online measurements.
func observeHandshakes(peers []wireguard.Peer, raw string, now int64) ([]PeerStatus, bool) {
	stamps := map[string]int64{}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, false
		}
		stamp, err := strconv.ParseInt(fields[1], 10, 64)
		if _, duplicate := stamps[fields[0]]; err != nil || stamp < 0 || duplicate {
			return nil, false
		}
		stamps[fields[0]] = stamp
	}
	if len(stamps) != len(peers) {
		return nil, false
	}
	out := make([]PeerStatus, 0, len(peers))
	for _, peer := range peers {
		stamp, ok := stamps[peer.PublicKey]
		if !ok {
			return nil, false
		}
		out = append(out, PeerStatus{PublicKey: peer.PublicKey, LastHandshake: stamp, Online: stamp > 0 && now >= stamp && now-stamp < 180})
	}
	return out, true
}

type journal struct {
	Current  *wireguard.ManagedConfig `json:"current"`
	Previous *wireguard.ManagedConfig `json:"previous"`
	Pending  *wireguard.ManagedConfig `json:"pending"`
}
type Engine struct {
	mu        sync.Mutex
	work      sync.Mutex
	run       Runner
	load      func() (wireguard.ManagedConfig, error)
	directory string
	record    journal
	status    Status
	active    bool
	jobs      sync.WaitGroup
}
type snapshot struct {
	addresses   []address
	routes      []route
	names       map[string]bool
	fingerprint string
	portUsed    bool
}
type address struct {
	Dev   string
	Local string
	Bits  int
}
type route struct {
	Dev   string
	Dst   string
	Table any
}

func hash(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func New(directory string, load func() (wireguard.ManagedConfig, error), run Runner) (*Engine, error) {
	if security.PrivateDirectory(directory, true) != nil {
		return nil, ErrUnavailable
	}
	e := &Engine{directory: directory, load: load, run: run, status: Status{Stage: "not_applied"}}
	path := filepath.Join(directory, "journal.json")
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		b, err := security.ReadPrivateFile(path, 256*1024)
		if err != nil || json.Unmarshal(b, &e.record) != nil {
			return nil, ErrUnavailable
		}
		for _, c := range []*wireguard.ManagedConfig{e.record.Current, e.record.Previous, e.record.Pending} {
			if c != nil && c.Validate() != nil {
				return nil, ErrUnavailable
			}
		}
	}
	if e.record.Pending != nil {
		e.status.Stage = "recovery_required"
		e.status.Error = "previous_operation_interrupted"
	} else if e.record.Current != nil {
		e.status.Stage = "applied_waiting_handshake"
	}
	return e, nil
}
func (e *Engine) save() error {
	b, err := json.Marshal(e.record)
	if err != nil {
		return ErrUnavailable
	}
	f, err := os.CreateTemp(e.directory, ".journal-")
	if err != nil {
		return ErrUnavailable
	}
	defer os.Remove(f.Name())
	if f.Chmod(0600) != nil {
		f.Close()
		return ErrUnavailable
	}
	if _, err = f.Write(b); err != nil {
		f.Close()
		return ErrUnavailable
	}
	if f.Sync() != nil {
		f.Close()
		return ErrUnavailable
	}
	if f.Close() != nil || os.Rename(f.Name(), filepath.Join(e.directory, "journal.json")) != nil {
		return ErrUnavailable
	}
	return syncDir(e.directory)
}
func syncDir(path string) error {
	if os.PathSeparator == '\\' {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return ErrUnavailable
	}
	defer f.Close()
	return f.Sync()
}
func (e *Engine) inspect(ctx context.Context, c wireguard.ManagedConfig) (snapshot, error) {
	commands := [][]string{{"ip", "-j", "address", "show"}, {"ip", "-j", "route", "show", "table", "all"}, {"ip", "-6", "-j", "route", "show", "table", "all"}, {"ip", "-j", "rule", "show"}, {"ip", "-6", "-j", "rule", "show"}, {"ss", "-H", "-lun", "sport", "=", ":" + strconv.Itoa(c.ListenPort)}}
	type result struct {
		i   int
		b   []byte
		err error
	}
	channel := make(chan result, len(commands))
	for i, args := range commands {
		go func(i int, args []string) { b, err := e.run(ctx, args[0], args[1:]...); channel <- result{i, b, err} }(i, args)
	}
	outputs := make([][]byte, len(commands))
	for range commands {
		r := <-channel
		if r.err != nil {
			return snapshot{}, ErrUnavailable
		}
		outputs[r.i] = r.b
	}
	s := snapshot{names: map[string]bool{}, portUsed: len(strings.TrimSpace(string(outputs[5]))) > 0}
	var links []struct {
		Ifname    string `json:"ifname"`
		Addresses []struct {
			Local  string `json:"local"`
			Prefix int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if json.Unmarshal(outputs[0], &links) != nil {
		return s, ErrUnavailable
	}
	stable := []string{}
	for _, link := range links {
		s.names[link.Ifname] = true
		stable = append(stable, "link:"+link.Ifname)
		for _, a := range link.Addresses {
			if a.Local != "" {
				s.addresses = append(s.addresses, address{link.Ifname, a.Local, a.Prefix})
				stable = append(stable, fmt.Sprintf("addr:%s:%s/%d", link.Ifname, a.Local, a.Prefix))
			}
		}
	}
	for i := 1; i < 5; i++ {
		var rows []map[string]any
		if json.Unmarshal(outputs[i], &rows) != nil {
			return s, ErrUnavailable
		}
		for _, row := range rows {
			clean := map[string]any{"kind": i}
			for _, key := range []string{"dst", "gateway", "dev", "table", "protocol", "scope", "type", "prefsrc", "metric", "priority", "src", "fwmark", "fwmask", "iif", "oif", "action", "suppress_prefixlength", "not", "uidrange"} {
				if value, ok := row[key]; ok {
					clean[key] = value
				}
			}
			b, _ := json.Marshal(clean)
			stable = append(stable, string(b))
			if i < 3 {
				dev, _ := row["dev"].(string)
				dst, _ := row["dst"].(string)
				s.routes = append(s.routes, route{dev, dst, row["table"]})
			}
		}
	}
	sort.Strings(stable)
	s.fingerprint = hash(stable)
	return s, nil
}
func (e *Engine) owned(ctx context.Context, c *wireguard.ManagedConfig, s snapshot) bool {
	if c == nil || !s.names[c.Interface] {
		return false
	}
	b, err := e.run(ctx, "wg", "show", c.Interface, "public-key")
	if err != nil || strings.TrimSpace(string(b)) != c.PublicKey {
		return false
	}
	b, err = e.run(ctx, "wg", "show", c.Interface, "listen-port")
	if err != nil || strings.TrimSpace(string(b)) != strconv.Itoa(c.ListenPort) {
		return false
	}
	b, err = e.run(ctx, "wg", "show", c.Interface, "allowed-ips")
	if err != nil {
		return false
	}
	actual := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return false
		}
		actual[fields[0]] = strings.Split(strings.Join(fields[1:], ""), ",")
	}
	expected := map[string][]string{}
	for _, p := range c.Peers {
		expected[p.PublicKey] = append([]string(nil), p.AllowedIPs...)
		sort.Strings(expected[p.PublicKey])
	}
	for key := range actual {
		sort.Strings(actual[key])
	}
	return hash(actual) == hash(expected)
}
func difference(a, b []string) []string {
	out := []string{}
	for _, v := range a {
		found := false
		for _, other := range b {
			if v == other {
				found = true
			}
		}
		if !found {
			out = append(out, v)
		}
	}
	return out
}
func hasRoute(s snapshot, iface, raw string) bool {
	wanted, _ := netip.ParsePrefix(raw)
	for _, r := range s.routes {
		actual, err := netip.ParsePrefix(r.Dst)
		if err != nil {
			if ip, err := netip.ParseAddr(r.Dst); err == nil {
				actual = netip.PrefixFrom(ip, ip.BitLen())
			}
		}
		if r.Dev == iface && actual == wanted {
			return true
		}
	}
	return false
}
func (e *Engine) plan(ctx context.Context) (Plan, wireguard.ManagedConfig, error) {
	c, err := e.load()
	if err != nil || c.Validate() != nil {
		return Plan{}, c, ErrUnavailable
	}
	s, err := e.inspect(ctx, c)
	if err != nil {
		return Plan{}, c, err
	}
	e.mu.Lock()
	current := e.record.Current
	pending := e.record.Pending
	e.mu.Unlock()
	if pending != nil {
		return Plan{}, c, ErrConflict
	}
	exists := s.names[c.Interface]
	owned := e.owned(ctx, current, s)
	if owned && (current.Interface != c.Interface || current.Address != c.Address || current.ListenPort != c.ListenPort || current.PublicKey != c.PublicKey) {
		return Plan{}, c, ErrConflict
	}
	if exists && !owned {
		return Plan{}, c, ErrConflict
	}
	if s.portUsed && !owned {
		return Plan{}, c, ErrConflict
	}
	if owned {
		for _, a := range s.addresses {
			if a.Dev == c.Interface && fmt.Sprintf("%s/%d", a.Local, a.Bits) != current.Address {
				return Plan{}, c, ErrConflict
			}
		}
		for _, r := range s.routes {
			if r.Dev != c.Interface {
				continue
			}
			allowed := false
			actual, err := netip.ParsePrefix(r.Dst)
			if err != nil {
				if ip, err := netip.ParseAddr(r.Dst); err == nil {
					actual = netip.PrefixFrom(ip, ip.BitLen())
				}
			}
			if actual.String() == current.Address {
				allowed = true
			}
			for _, raw := range current.Routes() {
				if actual.String() == raw {
					allowed = true
				}
			}
			if !allowed {
				return Plan{}, c, ErrConflict
			}
		}
		addressFound := false
		for _, a := range s.addresses {
			if a.Dev == c.Interface && fmt.Sprintf("%s/%d", a.Local, a.Bits) == c.Address {
				addressFound = true
			}
		}
		if !addressFound {
			return Plan{}, c, ErrConflict
		}
		for _, raw := range current.Routes() {
			found := false
			wanted, _ := netip.ParsePrefix(raw)
			for _, r := range s.routes {
				actual, err := netip.ParsePrefix(r.Dst)
				if err != nil {
					if ip, err := netip.ParseAddr(r.Dst); err == nil {
						actual = netip.PrefixFrom(ip, ip.BitLen())
					}
				}
				if r.Dev == c.Interface && actual == wanted {
					found = true
				}
			}
			if !found {
				return Plan{}, c, ErrConflict
			}
		}
	}
	local, _ := netip.ParsePrefix(c.Address)
	for _, peer := range c.Peers {
		if peer.Endpoint == "" {
			continue
		}
		host, _, _ := net.SplitHostPort(peer.Endpoint)
		if ip, err := netip.ParseAddr(host); err == nil {
			if local.Contains(ip) {
				return Plan{}, c, ErrConflict
			}
			for _, raw := range c.Routes() {
				route, _ := netip.ParsePrefix(raw)
				if route.Contains(ip) {
					return Plan{}, c, ErrConflict
				}
			}
		}
	}
	for _, a := range s.addresses {
		ip, err := netip.ParseAddr(a.Local)
		if err == nil && ip == local.Addr() && a.Dev != c.Interface {
			return Plan{}, c, ErrConflict
		}
	}
	for _, r := range s.routes {
		if r.Dev == c.Interface && owned {
			continue
		}
		if r.Dst == "default" || r.Dst == "" {
			continue
		}
		existing, err := netip.ParsePrefix(r.Dst)
		if err != nil {
			if ip, err := netip.ParseAddr(r.Dst); err == nil {
				existing = netip.PrefixFrom(ip, ip.BitLen())
			}
		}
		if existing.IsValid() && existing.Overlaps(local) {
			return Plan{}, c, ErrConflict
		}
	}
	for _, raw := range c.Routes() {
		p, _ := netip.ParsePrefix(raw)
		for _, r := range s.routes {
			if r.Dst == "default" || r.Dst == "" {
				continue
			}
			other, err := netip.ParsePrefix(r.Dst)
			if err != nil {
				ip, err := netip.ParseAddr(r.Dst)
				if err != nil {
					continue
				}
				other = netip.PrefixFrom(ip, ip.BitLen())
			}
			if p.Overlaps(other) && !(owned && r.Dev == c.Interface) {
				return Plan{}, c, ErrConflict
			}
		}
	}
	previous := []string{}
	if owned {
		previous = current.Routes()
	}
	p := Plan{DesiredDigest: hash(c), Config: c, ExistingOwned: owned, RoutesAdded: difference(c.Routes(), previous), RoutesRemoved: difference(previous, c.Routes())}
	p.Config.Secret = ""
	p.Digest = hash(struct {
		Desired  string
		Baseline string
	}{p.DesiredDigest, s.fingerprint})
	p.Applied = owned && hash(*current) == p.DesiredDigest
	return p, c, nil
}
func (e *Engine) Plan(ctx context.Context) (Plan, error) {
	if !e.work.TryLock() {
		return Plan{}, ErrConflict
	}
	defer e.work.Unlock()
	e.mu.Lock()
	active := e.active
	e.mu.Unlock()
	if active {
		return Plan{}, ErrConflict
	}
	p, _, err := e.plan(ctx)
	return p, err
}
func (e *Engine) withConfig(ctx context.Context, c wireguard.ManagedConfig) error {
	text, err := c.WireGuardText()
	if err != nil {
		return ErrUnavailable
	}
	f, err := os.CreateTemp(e.directory, ".wg-")
	if err != nil {
		return ErrUnavailable
	}
	defer os.Remove(f.Name())
	if f.Chmod(0600) != nil {
		f.Close()
		return ErrUnavailable
	}
	if _, err = f.WriteString(text); err != nil {
		f.Close()
		return ErrUnavailable
	}
	if f.Close() != nil {
		return ErrUnavailable
	}
	_, err = e.run(ctx, "wg", "syncconf", c.Interface, f.Name())
	return err
}
func routeArgs(action, raw, iface string) []string {
	args := []string{}
	p, _ := netip.ParsePrefix(raw)
	if p.Addr().Is6() {
		args = append(args, "-6")
	}
	return append(args, "route", action, raw, "dev", iface, "proto", "static")
}
func (e *Engine) execute(ctx context.Context, p Plan, c wireguard.ManagedConfig) (result error) {
	e.mu.Lock()
	beforeRecord := e.record
	before := e.record.Current
	e.record.Pending = &c
	err := e.save()
	if err != nil {
		e.record = beforeRecord
	}
	e.mu.Unlock()
	if err != nil {
		return err
	}
	created := false
	defer func() {
		if result != nil {
			rollbackCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			ok := true
			if created {
				if _, err := e.run(rollbackCtx, "ip", "link", "delete", "dev", c.Interface); err != nil {
					ok = false
				}
			}
			if !created && p.ExistingOwned && before != nil {
				old := *before
				old.Secret = c.Secret
				if e.withConfig(rollbackCtx, old) != nil {
					ok = false
				}
				observed, inspectError := e.inspect(rollbackCtx, c)
				if inspectError != nil {
					ok = false
				}
				for _, route := range p.RoutesAdded {
					if inspectError == nil && !hasRoute(observed, c.Interface, route) {
						continue
					}
					if _, err := e.run(rollbackCtx, "ip", routeArgs("delete", route, c.Interface)...); err != nil {
						ok = false
					}
				}
				for _, route := range p.RoutesRemoved {
					if inspectError == nil && hasRoute(observed, c.Interface, route) {
						continue
					}
					if _, err := e.run(rollbackCtx, "ip", routeArgs("add", route, c.Interface)...); err != nil {
						ok = false
					}
				}
			}
			e.mu.Lock()
			if ok {
				e.record = beforeRecord
				if e.save() != nil {
					e.status.Error = "rollback_journal_failed_manual_review_required"
				}
			} else {
				e.status.Error = "rollback_failed_manual_review_required"
			}
			e.mu.Unlock()
		}
	}()
	if !p.ExistingOwned {
		if _, err = e.run(ctx, "ip", "link", "add", "dev", c.Interface, "type", "wireguard"); err != nil {
			return err
		}
		created = true
	}
	if err = e.withConfig(ctx, c); err != nil {
		return err
	}
	if created {
		if _, err = e.run(ctx, "ip", "address", "add", c.Address, "dev", c.Interface); err != nil {
			return err
		}
		if _, err = e.run(ctx, "ip", "link", "set", "dev", c.Interface, "up"); err != nil {
			return err
		}
	}
	for _, route := range p.RoutesAdded {
		if _, err = e.run(ctx, "ip", routeArgs("add", route, c.Interface)...); err != nil {
			return err
		}
	}
	for _, route := range p.RoutesRemoved {
		if _, err = e.run(ctx, "ip", routeArgs("delete", route, c.Interface)...); err != nil {
			return err
		}
	}
	// Confirm effective routing, including policy rules, instead of changing them.
	for _, route := range c.Routes() {
		prefix, _ := netip.ParsePrefix(route)
		args := []string{}
		if prefix.Addr().Is6() {
			args = append(args, "-6")
		}
		args = append(args, "-j", "route", "get", prefix.Addr().String())
		b, err := e.run(ctx, "ip", args...)
		var result []struct {
			Dev string `json:"dev"`
		}
		if err != nil || json.Unmarshal(b, &result) != nil || len(result) != 1 || result[0].Dev != c.Interface {
			return ErrConflict
		}
	}
	e.mu.Lock()
	e.record = journal{Current: &c, Previous: before}
	err = e.save()
	e.mu.Unlock()
	return err
}

// Apply returns immediately. The bounded worker revalidates the exact reviewed
// plan before any mutation; HTTP client timeouts do not repeat a transaction.
func (e *Engine) Apply(digest string) (Status, error) {
	if len(digest) != 64 {
		return Status{}, ErrConflict
	}
	e.mu.Lock()
	if e.active || e.record.Pending != nil {
		e.mu.Unlock()
		return Status{}, ErrConflict
	}
	e.active = true
	e.jobs.Add(1)
	e.status = Status{Stage: "applying", Job: hash(time.Now().UnixNano())[:16]}
	initial := e.status
	e.mu.Unlock()
	go func() {
		defer e.jobs.Done()
		e.work.Lock()
		defer e.work.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		p, c, err := e.plan(ctx)
		if err == nil && p.Digest != digest {
			err = ErrConflict
		}
		if err == nil && !p.Applied {
			err = e.execute(ctx, p, c)
		}
		e.mu.Lock()
		defer e.mu.Unlock()
		e.active = false
		if err != nil {
			e.status.Stage = "failed"
			if e.status.Error == "" {
				e.status.Error = "apply_failed_or_plan_changed"
			}
		} else {
			e.status.Stage = "applied_waiting_handshake"
			e.status.DesiredDigest = p.DesiredDigest
		}
	}()
	return initial, nil
}
func (e *Engine) Status(ctx context.Context) Status {
	e.mu.Lock()
	out := e.status
	out.CheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
	out.HandshakeRecent = false
	out.ObservationAvailable = false
	out.Peers = nil
	current := e.record.Current
	out.CanRollback = current != nil || e.record.Pending != nil
	out.Applied = current != nil && !e.active && e.record.Pending == nil
	e.mu.Unlock()
	if !out.Applied {
		return out
	}
	desired, err := e.load()
	if err != nil || hash(desired) != hash(*current) {
		out.Applied = false
		out.Stage = "configuration_changed"
		return out
	}
	b, err := e.run(ctx, "wg", "show", current.Interface, "public-key")
	if err != nil || strings.TrimSpace(string(b)) != current.PublicKey {
		out.Applied = false
		out.Stage = "interface_absent_or_changed"
		return out
	}
	b, err = e.run(ctx, "ip", "-j", "link", "show", "dev", current.Interface)
	var links []struct {
		Flags []string `json:"flags"`
	}
	if err != nil || json.Unmarshal(b, &links) != nil || len(links) != 1 {
		out.Applied = false
		out.Stage = "interface_unavailable"
		return out
	}
	up := false
	for _, flag := range links[0].Flags {
		if flag == "UP" {
			up = true
		}
	}
	if !up {
		out.Applied = false
		out.Stage = "interface_down"
		return out
	}
	out.DesiredDigest = hash(*current)
	b, err = e.run(ctx, "wg", "show", current.Interface, "latest-handshakes")
	if err == nil {
		out.Peers, out.ObservationAvailable = observeHandshakes(current.Peers, string(b), time.Now().Unix())
		out.HandshakeRecent = out.ObservationAvailable && len(out.Peers) > 0
		for _, peer := range out.Peers {
			out.HandshakeRecent = out.HandshakeRecent && peer.Online
		}
		out.Stage = "applied_waiting_handshake"
		if out.HandshakeRecent {
			out.Stage = "handshake_recent"
		}
	}
	if !out.ObservationAvailable {
		out.Stage = "observation_unavailable"
	}
	return out
}

func (e *Engine) Rollback() (Status, error) {
	e.mu.Lock()
	if e.active || (e.record.Current == nil && e.record.Pending == nil) {
		e.mu.Unlock()
		return Status{}, ErrConflict
	}
	e.active = true
	e.jobs.Add(1)
	e.status = Status{Stage: "rolling_back", Job: hash(time.Now().UnixNano())[:16]}
	initial := e.status
	e.mu.Unlock()
	go func() {
		defer e.jobs.Done()
		e.work.Lock()
		defer e.work.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := func() error {
			e.mu.Lock()
			saved := e.record
			current := saved.Current
			previous := saved.Previous
			if saved.Pending != nil {
				current = saved.Pending
				previous = saved.Current
			}
			e.mu.Unlock()
			if current == nil {
				return ErrConflict
			}
			s, err := e.inspect(ctx, *current)
			if err != nil || !e.owned(ctx, current, s) {
				return ErrConflict
			}
			// A dedicated interface must not have acquired foreign addresses/routes.
			for _, a := range s.addresses {
				if a.Dev == current.Interface && fmt.Sprintf("%s/%d", a.Local, a.Bits) != current.Address {
					return ErrConflict
				}
			}
			for _, r := range s.routes {
				if r.Dev != current.Interface {
					continue
				}
				raw := r.Dst
				if !strings.Contains(raw, "/") {
					if ip, err := netip.ParseAddr(raw); err == nil {
						raw = netip.PrefixFrom(ip, ip.BitLen()).String()
					}
				}
				if raw != current.Address && len(difference([]string{raw}, current.Routes())) != 0 {
					return ErrConflict
				}
			}
			if previous == nil {
				if _, err = e.run(ctx, "ip", "link", "delete", "dev", current.Interface); err != nil {
					return err
				}
			} else {
				source, err := e.load()
				if err != nil || source.PublicKey != previous.PublicKey {
					return ErrConflict
				}
				old := *previous
				old.Secret = source.Secret
				if e.withConfig(ctx, old) != nil {
					return ErrUnavailable
				}
				for _, route := range difference(current.Routes(), old.Routes()) {
					if !hasRoute(s, current.Interface, route) {
						continue
					}
					if _, err = e.run(ctx, "ip", routeArgs("delete", route, current.Interface)...); err != nil {
						return err
					}
				}
				for _, route := range difference(old.Routes(), current.Routes()) {
					if hasRoute(s, current.Interface, route) {
						continue
					}
					if _, err = e.run(ctx, "ip", routeArgs("add", route, current.Interface)...); err != nil {
						return err
					}
				}
			}
			e.mu.Lock()
			defer e.mu.Unlock()
			e.record = journal{Current: previous}
			return e.save()
		}()
		e.mu.Lock()
		defer e.mu.Unlock()
		e.active = false
		if err != nil {
			e.status.Stage = "recovery_required"
			e.status.Error = "rollback_failed_manual_review_required"
		} else {
			e.status.Stage = "not_applied"
			if e.record.Current != nil {
				e.status.Stage = "previous_configuration_restored"
			}
		}
	}()
	return initial, nil
}

func (e *Engine) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { e.jobs.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
