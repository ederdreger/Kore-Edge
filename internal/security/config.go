// Package security guards the local control plane with explicit RBAC and mTLS.
package security

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

var ErrUnavailable = errors.New("security configuration unavailable")
var ErrDenied = errors.New("access denied")

type Config struct {
	Mode           string `yaml:"mode"`
	Certificate    string `yaml:"server_certificate"`
	PrivateKey     string `yaml:"server_private_key"`
	ClientCA       string `yaml:"client_ca"`
	Policy         string `yaml:"policy"`
	AuditDirectory string `yaml:"audit_directory"`
}

func (c Config) EffectiveMode() string {
	if c.Mode == "" {
		return "local_readonly"
	}
	return c.Mode
}
func (c Config) Validate() error {
	paths := []string{c.Certificate, c.PrivateKey, c.ClientCA, c.Policy, c.AuditDirectory}
	if c.EffectiveMode() == "local_readonly" {
		for _, p := range paths {
			if p != "" {
				return ErrUnavailable
			}
		}
		return nil
	}
	if c.EffectiveMode() != "mtls" {
		return ErrUnavailable
	}
	seen := map[string]bool{}
	for _, p := range paths {
		if !filepath.IsAbs(p) || seen[filepath.Clean(p)] || filepath.Dir(filepath.Clean(p)) == filepath.Clean(p) {
			return ErrUnavailable
		}
		seen[filepath.Clean(p)] = true
	}
	return nil
}
func CheckPrivileges() error {
	if runtime.GOOS == "linux" && os.Geteuid() == 0 {
		return errors.New("agent must run as an unprivileged user")
	}
	return nil
}
