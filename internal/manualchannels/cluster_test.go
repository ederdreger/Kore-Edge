package manualchannels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClusterUsesScopedHeaderPinnedCertificateAndNoRedirects(t *testing.T) {
	token := strings.Repeat("a", 48)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token || r.URL.RawQuery != "" {
			w.WriteHeader(401)
			return
		}
		if strings.HasSuffix(r.URL.Path, "redirect") {
			http.Redirect(w, r, "https://other.invalid/private", 302)
			return
		}
		w.Write([]byte("synthetic TS"))
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	hash := sha256.Sum256(server.Certificate().Raw)
	pin := hex.EncodeToString(hash[:])
	client := server.Client()
	transport := client.Transport.(*http.Transport).Clone()
	transport.TLSClientConfig = clusterTLS(pin)
	client.Transport = transport
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return ErrDenied }
	in := Input{URL: server.URL + "/stream.ts", Token: token, Protocol: "cluster"}
	reader, e := requestCluster(context.Background(), in, client, transport.CloseIdleConnections)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(reader)
	reader.Close()
	if e != nil || string(data) != "synthetic TS" {
		t.Fatal("cluster payload changed")
	}
	transport.TLSClientConfig = clusterTLS(strings.Repeat("0", 64))
	if _, e = requestCluster(context.Background(), in, client, transport.CloseIdleConnections); e == nil {
		t.Fatal("wrong certificate pin accepted")
	}
	transport.TLSClientConfig = clusterTLS(pin)
	in.URL = server.URL + "/redirect"
	if _, e = requestCluster(context.Background(), in, client, transport.CloseIdleConnections); e == nil {
		t.Fatal("cluster redirect accepted")
	}
}
func TestClusterInputKeepsPeerTokenPrivateAndRejectsReexport(t *testing.T) {
	s, id := processingFixture(t)
	token := strings.Repeat("a", 48)
	inputs := []Input{{Protocol: "cluster", URL: "https://peer.example/api/v1/cluster/synthetic/stream.ts", Token: token, Certificate: strings.Repeat("b", 64)}}
	if e := s.SaveProcessing(id, &inputs, nil, s.Revision()); e != nil {
		t.Fatal(e)
	}
	view, e := s.Processing(id)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), token) {
		t.Fatal("peer token exposed")
	}
	policy := InputPolicy{ClusterExport: true}
	if s.SaveProcessingPolicy(id, nil, nil, &policy, s.Revision()) == nil {
		t.Fatal("cluster loop/reexport permitted")
	}
	for _, raw := range []string{"http://peer.example/api/v1/cluster/synthetic/stream.ts", "https://127.0.0.1/api/v1/cluster/synthetic/stream.ts", "https://user:password@peer.example/api/v1/cluster/synthetic/stream.ts"} {
		v := inputs[0]
		v.URL = raw
		if _, e := normalizeInput(v); e == nil {
			t.Fatal("unsafe cluster endpoint accepted")
		}
	}
}
