package manualchannels

import (
	"context"
	"kore-edge/internal/mediaresources"
	"strings"
	"testing"
)

func TestConfiguredEncoderCapacityReleasesAndReportsActualLimit(t *testing.T) {
	s, err := NewWithEncodingLimits(t.TempDir(), nil, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	if s.Encoders(context.Background(), "").MaxConcurrent != 3 {
		t.Fatal("advertised capacity does not match reservations")
	}
	releases := []func(){}
	for i := 0; i < 3; i++ {
		r, e := s.reserveEncoder()
		if e != nil {
			t.Fatal(e)
		}
		releases = append(releases, r)
	}
	if _, e := s.reserveEncoder(); e != mediaresources.ErrTranscodeBusy {
		t.Fatal("capacity not enforced", e)
	}
	releases[0]()
	releases[0]()
	r, e := s.reserveEncoder()
	if e != nil {
		t.Fatal("release leaked", e)
	}
	r()
	for _, release := range releases[1:] {
		release()
	}
	args := strings.Join(configuredTranscodeArgs(advancedProfile(), s.encoderThreads), " ")
	if !strings.Contains(args, "-threads:v:0 0") || !strings.Contains(args, "-threads:v:1 0") {
		t.Fatal("auto workers not forwarded")
	}
	for _, c := range []struct{ concurrent, workers int }{{0, 4}, {129, 4}, {2, -1}, {2, 65}} {
		if _, e := NewWithEncodingLimits(t.TempDir(), nil, c.concurrent, c.workers); e == nil {
			t.Fatal("invalid limits accepted")
		}
	}
}
