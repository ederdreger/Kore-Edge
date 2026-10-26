package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	config Config
	root   *x509.Certificate
	key    *ecdsa.PrivateKey
	roots  *x509.CertPool
	policy Policy
}

func certificate(t *testing.T, parent *x509.Certificate, signer *ecdsa.PrivateKey, server, expired bool) ([]byte, []byte, *x509.Certificate) {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	c := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "SENSITIVE-SYNTHETIC-NAME"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		c.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		c.DNSNames = []string{"localhost"}
	}
	if expired {
		c.NotAfter = now.Add(-time.Minute)
	}
	der, e := x509.CreateCertificate(rand.Reader, c, parent, &key.PublicKey, signer)
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), parsed
}
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, e := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	root, e = x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	f := &fixture{root: root, key: key, roots: x509.NewCertPool(), policy: Policy{Version: 1}}
	f.roots.AddCert(root)
	f.config = Config{Mode: "mtls", Certificate: filepath.Join(dir, "server.pem"), PrivateKey: filepath.Join(dir, "key.pem"), ClientCA: filepath.Join(dir, "ca.pem"), Policy: filepath.Join(dir, "policy.json"), AuditDirectory: filepath.Join(dir, "audit")}
	cert, private, _ := certificate(t, root, key, true, false)
	writeTest(t, f.config.Certificate, cert)
	writeTest(t, f.config.PrivateKey, private)
	writeTest(t, f.config.ClientCA, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return f
}
func writeTest(t *testing.T, path string, b []byte) {
	t.Helper()
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func (f *fixture) client(t *testing.T, role string, expired bool) tls.Certificate {
	t.Helper()
	cert, key, parsed := certificate(t, f.root, f.key, false, expired)
	pair, e := tls.X509KeyPair(cert, key)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(parsed.Raw)
	id := sha256.Sum256([]byte(role))
	f.policy.Clients = append(f.policy.Clients, Client{ID: hex.EncodeToString(id[:16]), Fingerprint: hex.EncodeToString(sum[:]), Role: role})
	f.save(t)
	return pair
}
func (f *fixture) save(t *testing.T) {
	t.Helper()
	b, e := json.Marshal(f.policy)
	if e != nil {
		t.Fatal(e)
	}
	writeTest(t, f.config.Policy, b)
}
func transport(f *fixture, cert *tls.Certificate, max uint16) *http.Transport {
	c := &tls.Config{RootCAs: f.roots, MinVersion: tls.VersionTLS12, MaxVersion: max}
	if cert != nil {
		c.Certificates = []tls.Certificate{*cert}
	}
	return &http.Transport{TLSClientConfig: c}
}

func TestMTLSRolesRevocationAndAudit(t *testing.T) {
	f := newFixture(t)
	pairs := map[string]tls.Certificate{}
	for _, role := range []string{"viewer", "operator", "admin", "enroller", "metrics"} {
		pairs[role] = f.client(t, role, false)
	}
	guard, e := Load(f.config)
	if e != nil {
		t.Fatal(e)
	}
	defer guard.Close()
	calls := 0
	server := httptest.NewUnstartedServer(guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		p := PrincipalFrom(r.Context())
		if !p.Authenticated {
			t.Error("missing principal")
		}
		w.WriteHeader(204)
	})))
	server.TLS = guard.TLSConfig()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	for _, tc := range []struct {
		role, path, method string
		status             int
	}{
		{"viewer", "/healthz", "GET", 204}, {"viewer", "/api/v1/catalog", "PUT", 403},
		{"operator", "/api/v1/streams", "POST", 204}, {"operator", "/api/v1/catalog", "PUT", 403},
		{"admin", "/api/v1/catalog", "PUT", 204}, {"admin", "/api/v1/streams/test", "DELETE", 204},
		{"enroller", "/api/v1/provisioning/register", "POST", 204}, {"enroller", "/api/v1/catalog", "GET", 403},
		{"metrics", "/metrics", "GET", 204}, {"metrics", "/api/v1/streams", "GET", 403},
		{"admin", "/api/v1/streams?secret=SENSITIVE-QUERY", "POST", 400},
	} {
		t.Run(tc.role+tc.method+tc.path, func(t *testing.T) {
			pair := pairs[tc.role]
			tr := transport(f, &pair, 0)
			defer tr.CloseIdleConnections()
			client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
			req, _ := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader("SENSITIVE-BODY"))
			req.Header.Set("Authorization", "Bearer SENSITIVE-TOKEN")
			resp, e := client.Do(req)
			if e != nil {
				t.Fatal(e)
			}
			resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("got %d want %d", resp.StatusCode, tc.status)
			}
		})
	}
	for _, tc := range []struct {
		name string
		pair *tls.Certificate
		max  uint16
	}{{"missing", nil, 0}, {"old TLS", nil, tls.VersionTLS12}} {
		t.Run(tc.name, func(t *testing.T) {
			tr := transport(f, tc.pair, tc.max)
			defer tr.CloseIdleConnections()
			_, e := (&http.Client{Transport: tr, Timeout: 3 * time.Second}).Get(server.URL + "/healthz")
			if e == nil {
				t.Fatal("handshake accepted")
			}
		})
	}
	// Revocation is effective even on an established TLS connection.
	pair := pairs["viewer"]
	tr := transport(f, &pair, 0)
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 3 * time.Second}
	resp, e := client.Get(server.URL + "/healthz")
	if e != nil {
		t.Fatal(e)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	f.policy.Clients[0].Disabled = true
	f.save(t)
	resp, e = client.Get(server.URL + "/healthz")
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatal("revocation not applied")
	}
	writeTest(t, f.config.Policy, []byte("SENSITIVE-INVALID-POLICY"))
	resp, e = client.Get(server.URL + "/healthz")
	if e != nil {
		t.Fatal(e)
	}
	resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatal("invalid policy did not fail closed")
	}
	b, e := os.ReadFile(filepath.Join(f.config.AuditDirectory, "audit.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "SENSITIVE") || strings.Contains(string(b), "127.0.0.1") || strings.Contains(string(b), "/api/") {
		t.Fatal("audit exposed request data")
	}
	if !strings.Contains(string(b), `"phase":"authorized"`) || !strings.Contains(string(b), `"phase":"completed"`) {
		t.Fatal("missing audit pair")
	}
	if calls != 7 {
		t.Fatalf("unexpected effects %d", calls)
	}
}

func TestUnknownAndExpiredCertificates(t *testing.T) {
	f := newFixture(t)
	_ = f.client(t, "viewer", false)
	unknownCert, unknownKey, _ := certificate(t, f.root, f.key, false, false)
	unknown, e := tls.X509KeyPair(unknownCert, unknownKey)
	if e != nil {
		t.Fatal(e)
	}
	expired := f.client(t, "operator", true)
	foreign := newFixture(t).client(t, "admin", false)
	guard, e := Load(f.config)
	if e != nil {
		t.Fatal(e)
	}
	defer guard.Close()
	server := httptest.NewUnstartedServer(guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unauthorized handler invoked") })))
	server.TLS = guard.TLSConfig()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	for _, tc := range []struct {
		name      string
		pair      tls.Certificate
		handshake bool
	}{{"unknown pin", unknown, false}, {"expired", expired, true}, {"foreign CA", foreign, true}} {
		t.Run(tc.name, func(t *testing.T) {
			tr := transport(f, &tc.pair, 0)
			defer tr.CloseIdleConnections()
			req, _ := http.NewRequest("GET", server.URL+"/healthz", nil)
			req.Header.Set("X-Forwarded-Client-Cert", "SENSITIVE-FAKE-CERT")
			resp, e := (&http.Client{Transport: tr, Timeout: 3 * time.Second}).Do(req)
			if tc.handshake {
				if e == nil {
					resp.Body.Close()
					t.Fatal("invalid certificate accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			resp.Body.Close()
			if resp.StatusCode != 401 {
				t.Fatal(resp.StatusCode)
			}
		})
	}
}

func TestLocalReadOnlyAndOrigin(t *testing.T) {
	calls := 0
	h := Local().Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if PrincipalFrom(r.Context()).Authenticated {
			t.Error("local marked authenticated")
		}
		w.WriteHeader(204)
	}))
	for _, tc := range []struct {
		method, host, remote, origin, path string
		status                             int
	}{{"GET", "localhost:8080", "127.0.0.1:1234", "", "/healthz", 204}, {"POST", "localhost:8080", "127.0.0.1:1234", "", "/api/v1/streams", 403}, {"GET", "evil.test", "127.0.0.1:1234", "", "/healthz", 403}, {"GET", "localhost:8080", "192.0.2.1:1234", "", "/healthz", 403}, {"GET", "localhost:8080", "127.0.0.1:1234", "http://evil.test", "/healthz", 403}, {"GET", "localhost:8080", "127.0.0.1:1234", "", "/unknown", 404}} {
		req := httptest.NewRequest(tc.method, "http://"+tc.host+tc.path, nil)
		req.RemoteAddr = tc.remote
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("X-Forwarded-Client-Cert", "admin")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%+v: %d", tc, w.Code)
		}
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestAuditFailClosedAndExclusive(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "audit")
	a, e := openAudit(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if other, e := openAudit(dir); e == nil {
		other.Close()
		t.Fatal("concurrent writer accepted")
	}
	a.size = MaxAuditBytes
	if _, e := a.Begin(Event{}); e == nil {
		t.Fatal("full audit accepted")
	}
	a.size = 0
	a.file.Close()
	if a.Deny(Event{}) == nil {
		t.Fatal("write failure accepted")
	}
	if !a.broken {
		t.Fatal("audit failure not latched")
	}
}

func TestAuditRequiredBeforeMutationAndExpiryRechecked(t *testing.T) {
	f := newFixture(t)
	pair := f.client(t, "admin", false)
	guard, e := Load(f.config)
	if e != nil {
		t.Fatal(e)
	}
	defer guard.Close()
	leaf, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil {
		t.Fatal(e)
	}
	state := &tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, f.root}}}
	if _, e := f.policy.authenticate(state, leaf.NotAfter.Add(time.Second)); e == nil {
		t.Fatal("established session retained expired identity")
	}
	if _, e := f.policy.authenticate(state, leaf.NotBefore.Add(-time.Second)); e == nil {
		t.Fatal("future certificate accepted")
	}
	effects := 0
	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		effects++
		b, e := os.ReadFile(filepath.Join(f.config.AuditDirectory, "audit.jsonl"))
		if e != nil || !strings.Contains(string(b), `"phase":"authorized"`) {
			t.Error("mutation preceded durable audit")
		}
		w.WriteHeader(204)
	}))
	req := func() *http.Request {
		r := httptest.NewRequest("POST", "https://localhost:8080/api/v1/streams", nil)
		r.RemoteAddr = "127.0.0.1:1234"
		r.TLS = state
		return r
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req())
	if w.Code != 204 || effects != 1 {
		t.Fatal("authorized operation failed")
	}
	guard.audit.mu.Lock()
	guard.audit.size = MaxAuditBytes
	guard.audit.mu.Unlock()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req())
	if w.Code != 503 || effects != 1 {
		t.Fatal("mutation ran without audit capacity")
	}
}

func TestRequestConcurrencyLimit(t *testing.T) {
	guard := Local()
	for i := 0; i < cap(guard.slots); i++ {
		guard.slots <- struct{}{}
	}
	req := httptest.NewRequest("GET", "http://localhost:8080/healthz", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	guard.Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("capacity bypass") })).ServeHTTP(w, req)
	if w.Code != 429 {
		t.Fatal(w.Code)
	}
}

func TestPolicyAndFileValidation(t *testing.T) {
	f := newFixture(t)
	f.client(t, "viewer", false)
	for _, data := range []string{`{"version":1,"clients":[],"extra":true}`, `{"version":1,"clients":[]}`, `{"version":1,"clients":[]} {}`} {
		writeTest(t, f.config.Policy, []byte(data))
		if _, e := loadPolicy(f.config.Policy); e == nil {
			t.Fatal("invalid policy accepted")
		}
	}
	f.policy.Clients[0].Role = "root"
	f.save(t)
	if _, e := loadPolicy(f.config.Policy); e == nil {
		t.Fatal("unknown role accepted")
	}
	if (Config{Mode: "mtls"}).Validate() == nil || (Config{Mode: "local_readonly", Policy: "relative"}).Validate() == nil {
		t.Fatal("invalid config accepted")
	}
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	if privateDirectory(cwd, false) == nil {
		t.Fatal("checkout directory accepted")
	}
	if _, e := readPrivate(f.config.PrivateKey, 1); e == nil {
		t.Fatal("oversize key accepted")
	}
}
