package installer_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1" // Nginx SHA fixture only; installer uses bcrypt, never this format.
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"kore-edge/internal/app"
)

func TestGatewayHostValidationAndRendering(t *testing.T) {
	for _, tc := range []struct {
		host  string
		valid bool
	}{{"panel.example.com", true}, {"203.0.113.10", true}, {"2001:db8::10", true}, {"2001:db8:0:0:0:0:0:1", true}, {"http://bad/", false}, {"domain.test;injection", false}, {"example..com", false}, {"-bad.example", false}, {"999.1.1.1", false}, {"127.0.0.1", false}, {"0.0.0.0", false}, {"::1", false}, {"::", false}, {":::", false}, {"1::2::3", false}, {"2001:db8:", false}, {"localhost", false}} {
		command := exec.Command(bash(t), "-c", `source ../../scripts/gateway-lib.sh; valid_public_host "$1"`, "test-host", tc.host)
		if (command.Run() == nil) != tc.valid {
			t.Fatalf("host validation mismatch: %s", tc.host)
		}
	}
	command := exec.Command(bash(t), "-c", `source ../../scripts/gateway-lib.sh; render_gateway panel.example.com 8443 127.0.0.1:18080 0`)
	out, e := command.Output()
	if e != nil {
		t.Fatal(e)
	}
	config := string(out)
	for _, name := range []string{"client_body", "proxy", "fastcgi", "uwsgi", "scgi"} {
		if !strings.Contains(config, name+"_temp_path /var/cache/kore-edge/gateway/") {
			t.Fatal("temporary path not isolated", name)
		}
	}
	for _, required := range []string{"auth_basic_user_file", "ssl_certificate_key", "limit_except GET HEAD { deny all; }", "proxy_set_header Authorization \"\"", "proxy_set_header Origin \"\"", "access_log off", "proxy_pass http://127.0.0.1:18080"} {
		if !strings.Contains(config, required) {
			t.Fatal("gateway missing protection", required)
		}
	}

}

// Opt-in test uses an extracted Nginx binary and private temporary files. No
// package installation, systemd, public bind or host configuration is involved.
func TestAuthenticatedHTTPSGateway(t *testing.T) {
	nginx := os.Getenv("KORE_EDGE_NGINX")
	if nginx == "" {
		t.Skip("set KORE_EDGE_NGINX for real isolated HTTPS gateway test")
	}
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	if os.Mkdir(cache, 0700) != nil {
		t.Fatal("cache fixture")
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	private, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	files := map[string][]byte{"server.crt": certPEM, "server.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})}
	sum := sha1.Sum([]byte("synthetic-password"))
	files["htpasswd"] = []byte("viewer:{SHA}" + base64.StdEncoding.EncodeToString(sum[:]) + "\n")
	for name, data := range files {
		if os.WriteFile(filepath.Join(dir, name), data, 0600) != nil {
			t.Fatal("fixture write")
		}
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	backendDone := make(chan error, 1)
	go func() { backendDone <- app.Serve(ctx, listener) }()
	defer func() {
		cancel()
		if e := <-backendDone; e != nil {
			t.Error(e)
		}
	}()
	reservation, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	port := strconv.Itoa(reservation.Addr().(*net.TCPAddr).Port)
	reservation.Close()
	render := exec.Command(bash(t), "-c", `source ../../scripts/gateway-lib.sh; render_gateway panel.example.com "$1" "$2" 0`, "render", port, listener.Addr().String())
	data, e := render.Output()
	if e != nil {
		t.Fatal(e)
	}
	configuration := strings.ReplaceAll(string(data), "/etc/kore-edge/gateway", filepath.ToSlash(dir))
	configuration = strings.ReplaceAll(configuration, "/var/cache/kore-edge/gateway", filepath.ToSlash(cache))
	configuration = strings.ReplaceAll(configuration, "listen 0.0.0.0:", "listen 127.0.0.1:")
	path := filepath.Join(dir, "nginx.conf")
	if os.WriteFile(path, []byte(configuration), 0600) != nil {
		t.Fatal("configuration fixture")
	}
	check := exec.Command(nginx, "-t", "-q", "-c", path)
	if output, e := check.CombinedOutput(); e != nil {
		t.Fatal("Nginx validation failed", string(output))
	}
	for _, name := range []string{"client", "proxy", "fastcgi", "uwsgi", "scgi"} {
		info, e := os.Stat(filepath.Join(cache, name))
		if e != nil || !info.IsDir() {
			t.Fatal("Nginx temp directory missing from private cache", name)
		}
	}
	process := exec.Command(nginx, "-c", path, "-g", "daemon off;")
	if process.Start() != nil {
		t.Fatal("Nginx could not start")
	}
	defer func() {
		stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = exec.CommandContext(stopCtx, nginx, "-s", "quit", "-c", path).Run()
		done := make(chan error, 1)
		go func() { done <- process.Wait() }()
		select {
		case <-done:
		case <-stopCtx.Done():
			_ = process.Process.Kill()
			<-done
		}
	}()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("root fixture")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	base := "https://127.0.0.1:" + port
	deadline := time.Now().Add(5 * time.Second)
	for {
		response, e := client.Get(base + "/healthz")
		if e == nil {
			response.Body.Close()
			if response.StatusCode != 401 {
				t.Fatal("anonymous gateway access accepted")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway unavailable")
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, tc := range []struct {
		method, path, password string
		status                 int
	}{{"GET", "/healthz", "wrong", 401}, {"GET", "/healthz", "synthetic-password", 200}, {"GET", "/", "synthetic-password", 200}, {"POST", "/api/v1/streams", "synthetic-password", 403}} {
		request, e := http.NewRequest(tc.method, base+tc.path, nil)
		if e != nil {
			t.Fatal(e)
		}
		request.SetBasicAuth("viewer", tc.password)
		response, e := client.Do(request)
		if e != nil {
			t.Fatal("gateway request failed")
		}
		response.Body.Close()
		if response.StatusCode != tc.status {
			t.Fatal(fmt.Sprintf("gateway %s %s status=%d expected=%d", tc.method, tc.path, response.StatusCode, tc.status))
		}
	}
	// The dashboard loads assets and several APIs together; its initial burst
	// must not be mistaken for sustained abuse by the gateway rate limiter.
	for i := 0; i < 16; i++ {
		request, _ := http.NewRequest("GET", base+"/healthz", nil)
		request.SetBasicAuth("viewer", "synthetic-password")
		response, e := client.Do(request)
		if e != nil {
			t.Fatal("dashboard burst failed")
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatal("dashboard burst was throttled", response.StatusCode)
		}
	}
}

func TestGatewayTempRepairPreservesConfiguration(t *testing.T) {
	render := exec.Command(bash(t), "-c", `source ../../scripts/gateway-lib.sh; render_gateway panel.example.com 8443 127.0.0.1:18080 0`)
	want, e := render.Output()
	if e != nil {
		t.Fatal(e)
	}
	legacy := string(want)
	for _, name := range []string{"fastcgi", "uwsgi", "scgi"} {
		legacy = strings.ReplaceAll(legacy, "  "+name+"_temp_path /var/cache/kore-edge/gateway/"+name+";\n", "")
	}
	path := filepath.Join(t.TempDir(), "nginx.conf")
	if os.WriteFile(path, []byte(legacy), 0600) != nil {
		t.Fatal("legacy fixture")
	}
	patch := func() []byte {
		t.Helper()
		command := exec.Command(bash(t), "-c", `path=$1; if command -v cygpath >/dev/null 2>&1; then path=$(cygpath -u "$path"); fi; source ../../scripts/gateway-lib.sh; repair_gateway_temp_paths "$path"`, "patch", path)
		out, e := command.Output()
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	got := patch()
	if string(got) != string(want) {
		t.Fatal("repair changed configuration beyond missing temporary paths")
	}
	if os.WriteFile(path, got, 0600) != nil {
		t.Fatal("repaired fixture")
	}
	if string(patch()) != string(want) {
		t.Fatal("repair not idempotent")
	}
}

func TestGatewayAddressScope(t *testing.T) {
	for _, tc := range []struct{ host, scope string }{{"10.1.2.3", "IP privado IPv4"}, {"172.16.0.2", "IP privado IPv4"}, {"172.31.255.2", "IP privado IPv4"}, {"172.32.0.2", "IP IPv4 nao privado"}, {"192.168.1.4", "IP privado IPv4"}, {"100.64.0.3", "IP compartilhado CGNAT"}, {"100.128.0.3", "IP IPv4 nao privado"}, {"fd12::10", "IP privado IPv6 (ULA)"}, {"panel.example.com", "Dominio (alcance depende do DNS)"}} {
		command := exec.Command(bash(t), "-c", `source ../../scripts/gateway-lib.sh; valid_public_host "$1" && gateway_host_scope "$1"`, "scope", tc.host)
		out, e := command.Output()
		if e != nil || string(out) != tc.scope {
			t.Fatal("address classification failed", tc.host, string(out), e)
		}
	}
}

func TestGatewayValidatorMatchesServiceUserAndBindCapability(t *testing.T) {
	// A shell mock records argv only. No systemd operation occurs in this test.
	command := exec.Command(bash(t), "-c", `source ../../scripts/gateway-lib.sh; systemd-run(){ printf '%s\n' "$@"; }; validate_gateway_as_service /etc/kore-edge/gateway/nginx.conf`)
	out, e := command.Output()
	if e != nil {
		t.Fatal(e)
	}
	for _, expected := range []string{"--wait", "--collect", "--property=User=kore-edge", "--property=AmbientCapabilities=CAP_NET_BIND_SERVICE", "--property=CapabilityBoundingSet=CAP_NET_BIND_SERVICE", "--property=NoNewPrivileges=yes", "--property=TimeoutStartSec=20s", "/usr/sbin/nginx\n-t\n-q\n-c\n/etc/kore-edge/gateway/nginx.conf"} {
		if !strings.Contains(string(out), expected) {
			t.Fatal("validator missing service boundary", expected)
		}
	}
}
