package config

import (
	"reflect"
	"strings"
	"sync"
	"testing"
)

const valid = "version: 1\nmode: laboratory\nhttp:\n  listen: '127.0.0.1:18080'\n"

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		ok          bool
	}{
		{"valid", valid, true},
		{"ipv6", strings.ReplaceAll(valid, "127.0.0.1:18080", "[::1]:18080"), true},
		{"unknown", valid + "secret: value\n", false},
		{"version", strings.ReplaceAll(valid, "version: 1", "version: 2"), false},
		{"production", strings.ReplaceAll(valid, "laboratory", "production"), false},
		{"wildcard", strings.ReplaceAll(valid, "127.0.0.1", "0.0.0.0"), false},
		{"hostname", strings.ReplaceAll(valid, "127.0.0.1", "localhost"), false},
		{"port", strings.ReplaceAll(valid, "18080", "0"), false},
		{"missing", "version: 1\nmode: laboratory\n", false},
		{"duplicate", valid + "version: 1\n", false},
		{"multiple", valid + "---\n" + valid, false},
		{"oversize", strings.Repeat("x", MaxBytes+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tc.input))
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v, error=%v", tc.ok, err)
			}
		})
	}
}

func TestStoreRetainsValidSnapshot(t *testing.T) {
	var s Store
	if _, ok := s.Snapshot(); ok {
		t.Fatal("empty store ready")
	}
	c, err := Parse(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(c); err != nil {
		t.Fatal(err)
	}
	bad := c
	bad.Version = 2
	if s.Apply(bad) == nil {
		t.Fatal("invalid replacement accepted")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Apply(c)
			got, ok := s.Snapshot()
			if !ok || !reflect.DeepEqual(got, c) {
				t.Error("lost valid snapshot")
			}
		}()
	}
	wg.Wait()
}

func TestExample(t *testing.T) {
	if _, err := Load("../../configs/edge.example.yaml"); err != nil {
		t.Fatal(err)
	}
}
