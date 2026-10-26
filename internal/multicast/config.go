// Package multicast provides explicit-interface IPv4 laboratory outputs.
package multicast

import (
	"errors"
	"net"
	"net/netip"
	"regexp"
)

var ErrConfig = errors.New("invalid multicast configuration")

type Target struct {
	ChannelID string `yaml:"channel_id" json:"channel_id"`
	Interface string `yaml:"interface" json:"interface"`
	SourceIP  string `yaml:"source_ip" json:"source_ip"`
	Group     string `yaml:"group" json:"group"`
	Port      int    `yaml:"port" json:"port"`
	TTL       int    `yaml:"ttl" json:"ttl"`
}
type Config struct {
	Mode    string   `yaml:"mode" json:"mode"`
	Outputs []Target `yaml:"outputs" json:"outputs"`
}

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (t Target) Validate() error {
	source, e := netip.ParseAddr(t.SourceIP)
	group, e2 := netip.ParseAddr(t.Group)
	if !idPattern.MatchString(t.ChannelID) || len(t.Interface) == 0 || len(t.Interface) > 15 || t.Port < 1 || t.Port > 65535 || t.TTL != 1 || e != nil || !source.Is4() || !source.IsPrivate() || e2 != nil || !group.Is4() || !netip.MustParsePrefix("239.0.0.0/8").Contains(group) {
		return ErrConfig
	}
	for _, r := range t.Interface {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
			return ErrConfig
		}
	}
	return nil
}
func (c Config) Validate() error {
	if c.Mode == "" && len(c.Outputs) == 0 {
		return nil
	}
	if (c.Mode != "dry_run" && c.Mode != "send") || len(c.Outputs) == 0 || len(c.Outputs) > 4 {
		return ErrConfig
	}
	ids := map[string]bool{}
	destinations := map[string]bool{}
	for _, t := range c.Outputs {
		if t.Validate() != nil || ids[t.ChannelID] {
			return ErrConfig
		}
		key := net.JoinHostPort(t.Group, fmtPort(t.Port))
		if destinations[key] {
			return ErrConfig
		}
		ids[t.ChannelID] = true
		destinations[key] = true
	}
	return nil
}

type Interface struct {
	MTU          int      `json:"mtu"`
	PointToPoint bool     `json:"point_to_point"`
	Loopback     bool     `json:"loopback"`
	Name         string   `json:"name"`
	Index        int      `json:"index"`
	Up           bool     `json:"up"`
	Multicast    bool     `json:"multicast"`
	IPv4         []string `json:"ipv4"`
}

func Interfaces() ([]Interface, error) {
	all, e := net.Interfaces()
	if e != nil {
		return nil, errors.New("interface inventory unavailable")
	}
	result := []Interface{}
	for _, nic := range all {
		entry := Interface{Name: nic.Name, Index: nic.Index, MTU: nic.MTU, PointToPoint: nic.Flags&net.FlagPointToPoint != 0, Loopback: nic.Flags&net.FlagLoopback != 0, Up: nic.Flags&net.FlagUp != 0, Multicast: nic.Flags&net.FlagMulticast != 0, IPv4: []string{}}
		addrs, e := nic.Addrs()
		if e != nil {
			return nil, errors.New("interface inventory unavailable")
		}
		for _, a := range addrs {
			p, e := netip.ParsePrefix(a.String())
			if e == nil && p.Addr().Is4() {
				entry.IPv4 = append(entry.IPv4, p.Addr().String())
			}
		}
		result = append(result, entry)
	}
	return result, nil
}
func resolve(t Target, all []Interface) (int, error) {
	if t.Validate() != nil {
		return 0, ErrConfig
	}
	for _, nic := range all {
		if nic.Name != t.Interface {
			continue
		}
		if !nic.Up || !nic.Multicast || nic.Index <= 0 || nic.PointToPoint || nic.Loopback || nic.MTU < DatagramSize+28 {
			return 0, errors.New("selected interface is not up/multicast capable")
		}
		for _, ip := range nic.IPv4 {
			if ip == t.SourceIP {
				return nic.Index, nil
			}
		}
		return 0, errors.New("source IP does not belong to selected interface")
	}
	return 0, errors.New("selected interface unavailable")
}
