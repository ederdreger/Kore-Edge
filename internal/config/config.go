// Package config owns the versioned laboratory configuration.
package config

import (
	"bytes"
	"fmt"
	"io"
	"kore-edge/internal/catalog"
	"kore-edge/internal/hls"
	"kore-edge/internal/integration"
	"kore-edge/internal/multicast"
	"kore-edge/internal/provision"
	"kore-edge/internal/security"
	"net"
	"os"
	"strconv"
	"sync"

	"gopkg.in/yaml.v3"
)

const MaxBytes = 64 * 1024

type Config struct {
	Integration integration.Config `yaml:"integration"`
	Security    security.Config    `yaml:"security"`
	Provision   provision.Config   `yaml:"provision"`
	HLS         hls.Config         `yaml:"hls"`
	Multicast   multicast.Config   `yaml:"multicast"`
	Stream      struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"stream"`
	Catalog struct {
		PartnerID string `yaml:"partner_id"`
		PublicKey string `yaml:"public_key"`
	} `yaml:"catalog"`
	Version int    `yaml:"version"`
	Mode    string `yaml:"mode"`
	HTTP    struct {
		Listen string `yaml:"listen"`
	} `yaml:"http"`
}

func (c Config) Validate() error {
	if c.Integration.Validate() != nil || c.Integration.Enabled && c.Catalog.PartnerID == "" {
		return fmt.Errorf("invalid integration configuration")
	}
	if c.Integration.Enabled && c.Provision.Enabled && c.Integration.EdgeID != c.Provision.EdgeID {
		return fmt.Errorf("integration identity mismatch")
	}
	if err := c.Security.Validate(); err != nil {
		return err
	}
	if c.Provision.Validate() != nil {
		return fmt.Errorf("invalid provisioning configuration")
	}
	if c.HLS.Validate() != nil {
		return fmt.Errorf("invalid HLS configuration")
	}
	if c.HLS.Enabled && !c.Stream.Enabled {
		return fmt.Errorf("HLS requires stream engine")
	}
	if err := c.Multicast.Validate(); err != nil {
		return fmt.Errorf("invalid multicast configuration")
	}
	if len(c.Multicast.Outputs) > 0 && !c.Stream.Enabled {
		return fmt.Errorf("multicast requires stream engine")
	}
	if c.Stream.Enabled && c.Catalog.PartnerID == "" {
		return fmt.Errorf("stream engine requires catalog trust configuration")
	}
	if c.Catalog.PartnerID != "" || c.Catalog.PublicKey != "" {
		if _, e := catalog.New(c.Catalog.PartnerID, c.Catalog.PublicKey); e != nil {
			return fmt.Errorf("invalid catalog trust configuration")
		}
	}
	if c.Version != 1 {
		return fmt.Errorf("unsupported configuration version")
	}
	if c.Mode != "laboratory" {
		return fmt.Errorf("only laboratory mode is supported")
	}
	host, port, err := net.SplitHostPort(c.HTTP.Listen)
	if err != nil {
		return fmt.Errorf("http.listen requires an explicit IP and port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("http.listen must use a literal loopback IP")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("http.listen port must be between 1 and 65535")
	}
	return nil
}

func Parse(r io.Reader) (Config, error) {
	var c Config
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return c, fmt.Errorf("read configuration: %w", err)
	}
	if len(data) > MaxBytes {
		return c, fmt.Errorf("configuration exceeds size limit")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	// Do not propagate YAML errors: they may contain user supplied values.
	if err := d.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("invalid YAML configuration")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return Config{}, fmt.Errorf("expected one YAML document")
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("cannot open configuration file")
	}
	defer f.Close()
	return Parse(f)
}

// Store publishes complete validated snapshots. Invalid replacements retain the
// last valid snapshot. Persistence and live listener reload are outside phase 01.
type Store struct {
	mu      sync.RWMutex
	current Config
	valid   bool
}

func (s *Store) Apply(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c.Multicast.Outputs = append([]multicast.Target(nil), c.Multicast.Outputs...)
	s.current, s.valid = c, true
	return nil
}

func (s *Store) Snapshot() (Config, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.current
	c.Multicast.Outputs = append([]multicast.Target(nil), c.Multicast.Outputs...)
	return c, s.valid
}
