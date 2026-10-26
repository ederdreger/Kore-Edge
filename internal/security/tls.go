package security

import (
	"crypto/tls"
	"crypto/x509"
)

type Controller struct {
	mode       string
	tls        *tls.Config
	policyPath string
	audit      *Audit
	slots      chan struct{}
}

func Local() *Controller { return &Controller{mode: "local_readonly", slots: make(chan struct{}, 32)} }
func Load(c Config) (*Controller, error) {
	if c.Validate() != nil {
		return nil, ErrUnavailable
	}
	if c.EffectiveMode() == "local_readonly" {
		return Local(), nil
	}
	cert, e := readPrivate(c.Certificate, 64*1024)
	if e != nil {
		return nil, e
	}
	key, e := readPrivate(c.PrivateKey, 16*1024)
	if e != nil {
		return nil, e
	}
	pair, e := tls.X509KeyPair(cert, key)
	for i := range key {
		key[i] = 0
	}
	if e != nil {
		return nil, ErrUnavailable
	}
	ca, e := readPrivate(c.ClientCA, 64*1024)
	if e != nil {
		return nil, e
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, ErrUnavailable
	}
	if _, e := loadPolicy(c.Policy); e != nil {
		return nil, e
	}
	audit, e := openAudit(c.AuditDirectory)
	if e != nil {
		return nil, e
	}
	return &Controller{mode: "mtls", tls: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}, policyPath: c.Policy, audit: audit, slots: make(chan struct{}, 32)}, nil
}
func (c *Controller) TLSConfig() *tls.Config {
	if c == nil || c.tls == nil {
		return nil
	}
	return c.tls.Clone()
}
func (c *Controller) Mode() string {
	if c == nil {
		return "local_readonly"
	}
	return c.mode
}
func (c *Controller) PermissionsVerified() bool {
	return c != nil && c.mode == "mtls" && permissionsVerified()
}
func (c *Controller) Close() error {
	if c != nil && c.audit != nil {
		return c.audit.Close()
	}
	return nil
}
