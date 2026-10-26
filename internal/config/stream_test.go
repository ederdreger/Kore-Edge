package config

import (
	"strings"
	"testing"
)

func TestStreamRequiresCatalog(t *testing.T) {
	if _, err := Parse(strings.NewReader(valid + "stream:\n  enabled: true\n")); err == nil {
		t.Fatal("enabled without trust")
	}
}
