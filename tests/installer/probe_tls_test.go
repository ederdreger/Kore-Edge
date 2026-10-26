package installer_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPanelProbeTLSCertificate(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{Subject: pkix.Name{CommonName: "Kore Edge synthetic TLS"}, BasicConstraintsValid: true, IsCA: true, SerialNumber: big.NewInt(42), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	private, _ := x509.MarshalPKCS8PrivateKey(key)
	certificate, e := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	path := filepath.Join(t.TempDir(), "server.crt")
	os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "https://"))
	command := exec.Command(bash(t), "-c", `source ../../scripts/gateway-lib.sh; certificate=$2; if command -v cygpath >/dev/null 2>&1; then certificate=$(cygpath -u "$certificate"); fi; check_panel_https 127.0.0.1 "$1" "$certificate" 1 0 || { printf '%s\n' "$GATEWAY_CHECK_ERROR"; curl -q --noproxy '*' --cacert "$certificate" --silent --show-error -o /dev/null "https://127.0.0.1:$1/api/v1/auth/me"; exit 1; }`, "probe", port, path)
	if out, e := command.CombinedOutput(); e != nil {
		t.Fatal(string(out), e)
	}
}
