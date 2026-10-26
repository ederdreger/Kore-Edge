// Package wireguard plans laboratory changes; it never applies them.
package wireguard

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

type Peer struct {
	PublicKey  string   `json:"public_key"`
	Endpoint   string   `json:"endpoint"`
	AllowedIPs []string `json:"allowed_ips"`
	Keepalive  int      `json:"keepalive_seconds"`
}
type Config struct {
	Interface  string `json:"interface"`
	Address    string `json:"address"`
	ListenPort int    `json:"listen_port"`
	Peers      []Peer `json:"peers"`
}
type Request struct {
	Before  *Config `json:"before"`
	Desired Config  `json:"desired"`
}
type Plan struct {
	Mode             string   `json:"mode"`
	Applied          bool     `json:"applied"`
	Interface        string   `json:"interface"`
	Template         string   `json:"public_template"`
	RollbackTemplate string   `json:"rollback_public_template,omitempty"`
	RollbackAction   string   `json:"rollback_action"`
	Routes           []string `json:"allowed_routes"`
	Preconditions    []string `json:"preconditions"`
	Validation       []string `json:"post_validation"`
}

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,14}$`)
var privateRanges = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("172.16.0.0/12"), netip.MustParsePrefix("192.168.0.0/16"), netip.MustParsePrefix("fc00::/7")}

func privatePrefix(p netip.Prefix) bool {
	for _, block := range privateRanges {
		if p.Addr().BitLen() == block.Addr().BitLen() && p.Bits() >= block.Bits() && block.Contains(p.Addr()) {
			return true
		}
	}
	return false
}
func validKey(key string) bool {
	b, e := base64.StdEncoding.DecodeString(key)
	if e != nil || len(b) != 32 || base64.StdEncoding.EncodeToString(b) != key {
		return false
	}
	for _, v := range b {
		if v != 0 {
			return true
		}
	}
	return false
}

func (c Config) Validate() error {
	if !namePattern.MatchString(c.Interface) || c.ListenPort < 1 || c.ListenPort > 65535 {
		return errors.New("explicit interface and valid listen port required")
	}
	address, e := netip.ParsePrefix(c.Address)
	if e != nil || !privatePrefix(address) {
		return errors.New("interface address must be private laboratory CIDR")
	}
	if len(c.Peers) == 0 || len(c.Peers) > 128 {
		return errors.New("between 1 and 128 peers required")
	}
	keys := map[string]bool{}
	var routes []netip.Prefix
	for _, peer := range c.Peers {
		if !validKey(peer.PublicKey) || keys[peer.PublicKey] {
			return errors.New("invalid or duplicate public key")
		}
		keys[peer.PublicKey] = true
		ep, e := netip.ParseAddrPort(peer.Endpoint)
		if e != nil || ep.Port() == 0 || ep.Addr().Zone() != "" || (!ep.Addr().IsPrivate() && !ep.Addr().IsLoopback()) || ep.Addr().Is4In6() {
			return errors.New("endpoint requires literal laboratory IP and explicit port")
		}
		if peer.Keepalive < 0 || peer.Keepalive > 65535 || len(peer.AllowedIPs) == 0 || len(peer.AllowedIPs) > 128 {
			return errors.New("invalid keepalive or allowed IP count")
		}
		for _, raw := range peer.AllowedIPs {
			p, e := netip.ParsePrefix(raw)
			if e != nil || p != p.Masked() || !privatePrefix(p) {
				return errors.New("allowed IPs require canonical private laboratory CIDRs; default routes forbidden")
			}
			if p.Contains(address.Addr()) {
				return errors.New("peer route contains local tunnel address")
			}
			for _, other := range routes {
				if p.Overlaps(other) {
					return errors.New("overlapping peer routes forbidden")
				}
			}
			routes = append(routes, p)
		}
	}
	return nil
}

// Render deliberately omits PrivateKey and PSK. This is a public review template,
// not an operational configuration; Table=off prevents automatic route creation.
func Render(c Config) (string, error) {
	if e := c.Validate(); e != nil {
		return "", e
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# PUBLIC REVIEW TEMPLATE - incomplete, do not apply\n[Interface]\nAddress = %s\nListenPort = %d\nTable = off\n# Private key must be supplied outside this API after operational approval.\n", c.Address, c.ListenPort)
	for _, p := range c.Peers {
		fmt.Fprintf(&b, "\n[Peer]\nPublicKey = %s\nEndpoint = %s\nAllowedIPs = %s\nPersistentKeepalive = %d\n", p.PublicKey, p.Endpoint, strings.Join(p.AllowedIPs, ", "), p.Keepalive)
	}
	return b.String(), nil
}

func DryRun(r Request) (Plan, error) {
	template, e := Render(r.Desired)
	if e != nil {
		return Plan{}, e
	}
	p := Plan{Mode: "dry_run", Interface: r.Desired.Interface, Template: template, Routes: []string{}, RollbackAction: "remove only the newly approved laboratory interface and its explicitly installed routes; preserve pre-existing state",
		Preconditions: []string{"confirm isolated laboratory host and operator-approved interface, addresses, ports and peers", "inspect existing interfaces, routes, policy rules and default route; abort on conflicts", "capture previous public configuration and route state; retain secrets separately outside Git/logs", "obtain specific operational approval; this plan performs no mutation"},
		Validation:    []string{"compare default routes and policy rules with baseline; they must be unchanged", "verify peer handshake and explicit allowed routes on laboratory host", "verify unrelated interfaces and streams are unchanged"}}
	if r.Before != nil {
		if r.Before.Interface != r.Desired.Interface {
			return Plan{}, errors.New("rollback interface must match desired interface")
		}
		p.RollbackTemplate, e = Render(*r.Before)
		if e != nil {
			return Plan{}, errors.New("invalid rollback baseline")
		}
		p.RollbackAction = "restore approved previous configuration and exact route baseline, including original interface state; retain original secret material separately"
	}
	for _, peer := range r.Desired.Peers {
		p.Routes = append(p.Routes, peer.AllowedIPs...)
	}
	return p, nil
}
