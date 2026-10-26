package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kore-edge/internal/provision"
)

func TestOfflineIssuerNeverPrintsToken(t *testing.T) {
	dir := t.TempDir()
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(provision.Assignment{EdgeID: "edge-lab", PartnerID: "partner-lab", CatalogPublicKey: base64.StdEncoding.EncodeToString(pub), TTLSeconds: 60})
	path := filepath.Join(dir, "delivery.json")
	var out bytes.Buffer
	if run([]string{"-action", "issue", "-directory", dir, "-token-out", path}, bytes.NewReader(body), &out) != nil {
		t.Fatal("issue")
	}
	secret, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var credential provision.Credential
	if json.Unmarshal(secret, &credential) != nil || credential.Token == "" {
		t.Fatal("delivery")
	}
	if strings.Contains(out.String(), credential.Token) {
		t.Fatal("raw bearer printed")
	}
	original := append([]byte(nil), secret...)
	if run([]string{"-action", "issue", "-directory", dir, "-token-out", path}, bytes.NewReader(body), &out) == nil {
		t.Fatal("credential file overwritten")
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(original, got) {
		t.Fatal("credential changed")
	}
	out.Reset()
	if run([]string{"-action", "revoke", "-directory", dir, "-token-id", credential.ID}, strings.NewReader(""), &out) != nil {
		t.Fatal("revoke")
	}
	if !strings.Contains(out.String(), "revoked") {
		t.Fatal("revoke receipt")
	}
}
