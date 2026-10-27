// Package capacity loads explicitly reviewed service admission limits.
package capacity

import (
	"fmt"
	"strconv"
)

type Limits struct{ Channels, HLSInputs, HLSOutputs int }

func Load(getenv func(string) string) (Limits, error) {
	l := Limits{16, 2, 2}
	for _, item := range []struct {
		name    string
		value   *int
		maximum int
	}{
		{"KORE_EDGE_MAX_CHANNELS", &l.Channels, 512},
		{"KORE_EDGE_HLS_INPUTS", &l.HLSInputs, 512},
		{"KORE_EDGE_HLS_OUTPUTS", &l.HLSOutputs, 128},
	} {
		if raw := getenv(item.name); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > item.maximum {
				return Limits{}, fmt.Errorf("invalid capacity: %s must be 1..%d", item.name, item.maximum)
			}
			*item.value = n
		}
	}
	return l, nil
}
