// Package manualchannels receives operator-owned URLs independently of the
// signed partner catalog. Secrets are confined to private storage and memory.
package manualchannels

import (
	"errors"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalid     = errors.New("invalid_manual_source")
	ErrConflict    = errors.New("manual_source_changed")
	ErrUnavailable = errors.New("manual_source_unavailable")
	ErrDenied      = errors.New("manual_source_denied")
)

type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	URL       string `json:"url"`
	Protocol  string `json:"protocol"`
	Interface string `json:"interface,omitempty"`
	LocalIP   string `json:"local_ip,omitempty"`
	Enabled   bool   `json:"enabled"`
}
type Detection struct {
	Protocol string `json:"protocol"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Verified bool   `json:"verified"`
	Detail   string `json:"detail"`
}

func parse(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 4096 || strings.ContainsAny(raw, "\r\n\x00") {
		return nil, ErrInvalid
	}
	// Flussonic displays an unescaped #!:: stream ID. It is an SRT query
	// value, not a URI fragment, and must survive parsing byte for byte.
	if strings.HasPrefix(strings.ToLower(raw), "srt://") {
		raw = strings.Replace(raw, "streamid=#!::", "streamid=%23!::", 1)
	}
	u, e := url.Parse(raw)
	if e != nil || u.Opaque != "" || u.Fragment != "" || u.Hostname() == "" || u.ForceQuery {
		return nil, ErrInvalid
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if !strings.Contains("|http|https|udp|srt|", "|"+u.Scheme+"|") {
		return nil, ErrInvalid
	}
	if u.Scheme != "udp" {
		if ip, e := netip.ParseAddr(u.Hostname()); e == nil && !permitted(ip.Unmap()) {
			return nil, ErrDenied
		}
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 {
			return nil, ErrInvalid
		}
	} else if u.Scheme == "srt" || u.Scheme == "udp" {
		return nil, ErrInvalid
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return nil, ErrInvalid
	}
	if u.Scheme == "srt" {
		if u.User != nil || u.Path != "" {
			return nil, ErrInvalid
		}
		for key, values := range q {
			if len(values) != 1 || !strings.Contains("|mode|streamid|passphrase|pbkeylen|latency|rcvlatency|peerlatency|transtype|pkt_size|", "|"+key+"|") {
				return nil, ErrInvalid
			}
		}
		if mode := q.Get("mode"); mode != "" && mode != "caller" {
			return nil, ErrInvalid
		}
		if mode := q.Get("transtype"); mode != "" && mode != "live" {
			return nil, ErrInvalid
		}
		if size := q.Get("pkt_size"); size != "" && size != "1316" {
			return nil, ErrInvalid
		}
		if len(q.Get("streamid")) > 512 {
			return nil, ErrInvalid
		}
		if key := q.Get("passphrase"); key != "" && (len(key) < 10 || len(key) > 79) {
			return nil, ErrInvalid
		}
		if key := q.Get("pbkeylen"); key != "" && key != "16" && key != "24" && key != "32" {
			return nil, ErrInvalid
		}
		for _, key := range []string{"latency", "rcvlatency", "peerlatency"} {
			if v := q.Get(key); v != "" {
				n, e := strconv.Atoi(v)
				if e != nil || n < 20000 || n > 5000000 {
					return nil, ErrInvalid
				}
			}
		}
	}
	if u.Scheme == "udp" && (u.RawQuery != "" || u.Path != "" || (u.User != nil && u.User.String() != "")) {
		return nil, ErrInvalid
	}
	if len(u.String()) > 4096 {
		return nil, ErrInvalid
	}
	return u, nil
}
func safeAddress(u *url.URL) string {
	c := *u
	c.User = nil
	c.RawQuery = ""
	c.Fragment = ""
	return c.String()
}
func Detect(raw, chosen string) (Detection, error) {
	u, e := parse(raw)
	if e != nil {
		return Detection{}, e
	}
	d := Detection{Address: safeAddress(u), Detail: "Formato identificado; sinal ainda não verificado."}
	switch u.Scheme {
	case "udp", "srt":
		d.Protocol = u.Scheme
	default:
		p := strings.ToLower(u.Path)
		if strings.HasSuffix(p, ".m3u8") {
			d.Protocol = "hls"
		} else if strings.HasSuffix(p, "/mpegts") || strings.HasSuffix(p, ".ts") {
			d.Protocol = "http_ts"
		}
		if chosen != "" && chosen != "auto" {
			if chosen != "hls" && chosen != "http_ts" {
				return Detection{}, ErrInvalid
			}
			d.Protocol = chosen
		}
	}
	if (u.Scheme == "udp" || u.Scheme == "srt") && chosen != "" && chosen != "auto" && chosen != u.Scheme {
		return Detection{}, ErrInvalid
	}
	name := path.Base(strings.TrimSuffix(u.Path, "/"))
	if name == "mpegts" || name == "index.m3u8" || name == "stream.ts" {
		name = path.Base(path.Dir(u.Path))
	}
	if u.Scheme == "srt" {
		for _, field := range strings.Split(u.Query().Get("streamid"), ",") {
			if i := strings.Index(field, "r="); i >= 0 {
				name = field[i+2:]
				break
			}
		}
	}
	if name == "." || name == "/" || name == "" {
		name = u.Hostname()
	}
	if len(name) > 128 {
		name = u.Hostname()
	}
	d.Name = name
	if d.Protocol == "" {
		d.Detail = "URL HTTP sem formato evidente. Clique em Verificar sinal ou escolha MPEG-TS/HLS."
	}
	return d, nil
}
func normalize(s Source) (Source, error) {
	d, e := Detect(s.URL, s.Protocol)
	if e != nil || d.Protocol == "" {
		return Source{}, ErrInvalid
	}
	u, _ := parse(s.URL)
	s.URL = u.String()
	s.Protocol = d.Protocol
	s.Name = strings.TrimSpace(s.Name)
	s.Category = strings.TrimSpace(s.Category)
	if s.Name == "" {
		s.Name = d.Name
	}
	if len(s.Name) > 128 || len(s.Category) > 128 || !utf8.ValidString(s.Name+s.Category) || strings.ContainsAny(s.Name+s.Category, "\r\n\x00") {
		return Source{}, ErrInvalid
	}
	if s.Protocol != "udp" && (s.Interface != "" || s.LocalIP != "") {
		return Source{}, ErrInvalid
	}
	if s.Protocol == "udp" {
		ip, e := netip.ParseAddr(u.Hostname())
		if e != nil || !ip.Is4() || ip.Is4In6() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsLoopback() || s.Interface == "" || len(s.Interface) > 32 {
			return Source{}, ErrInvalid
		}
		if ip.IsMulticast() && !netip.MustParsePrefix("239.0.0.0/8").Contains(ip) {
			return Source{}, ErrInvalid
		}
	}
	return s, nil
}
