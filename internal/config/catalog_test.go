package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCatalogTrustConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, tc := range []struct {
		yaml string
		ok   bool
	}{
		{"catalog:\n  partner_id: partner-lab\n  public_key: '" + key + "'\n", true},
		{"catalog:\n  partner_id: partner-lab\n", false},
		{"catalog:\n  public_key: '" + key + "'\n", false},
		{"catalog:\n  partner_id: partner-lab\n  public_key: invalid\n", false},
		{"catalog:\n  private_key: DO_NOT_ECHO\n", false},
	} {
		_, e := Parse(strings.NewReader(valid + tc.yaml))
		if (e == nil) != tc.ok {
			t.Fatalf("unexpected validation: %v", e)
		}
		if e != nil && strings.Contains(e.Error(), "DO_NOT_ECHO") {
			t.Fatal("secret in error")
		}
	}
}
