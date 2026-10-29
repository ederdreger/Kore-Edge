// Package capacity loads explicitly reviewed service admission limits.
package capacity

import (
	"fmt"
	"runtime"
	"strconv"
)

type Limits struct{ Channels, HLSInputs, HLSOutputs, Transcoders, EncoderThreads int }

func Load(getenv func(string) string) (Limits, error) {
	return load(getenv, runtime.NumCPU())
}

func load(getenv func(string) string, cpus int) (Limits, error) {
	// A starting process budget, not a guarantee of real-time throughput.
	// GOMAXPROCS controls Go and is not an encoder CPU allocation.
	l := Limits{16, 2, 2, max(1, min(32, cpus/4)), 0}
	for _, item := range []struct {
		name    string
		value   *int
		maximum int
	}{
		{"KORE_EDGE_MAX_CHANNELS", &l.Channels, 512},
		{"KORE_EDGE_HLS_INPUTS", &l.HLSInputs, 512},
		{"KORE_EDGE_HLS_OUTPUTS", &l.HLSOutputs, 128},
		{"KORE_EDGE_TRANSCODERS", &l.Transcoders, 128},
	} {
		if raw := getenv(item.name); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > item.maximum {
				return Limits{}, fmt.Errorf("invalid capacity: %s must be 1..%d", item.name, item.maximum)
			}
			*item.value = n
		}
	}
	if raw := getenv("KORE_EDGE_ENCODER_THREADS"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil || n < 0 || n > 64 {
			return Limits{}, fmt.Errorf("invalid capacity: KORE_EDGE_ENCODER_THREADS must be 0..64")
		}
		l.EncoderThreads = n // 0 delegates worker selection to the encoder.
	}
	return l, nil
}
