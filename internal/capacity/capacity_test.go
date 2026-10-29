package capacity

import "testing"

func TestEncodingCapacityScalesAndAcceptsExplicitAutomaticWorkers(t *testing.T) {
	for _, c := range []struct{ cpus, want int }{{1, 1}, {8, 2}, {28, 7}, {32, 8}, {256, 32}} {
		v, e := load(func(string) string { return "" }, c.cpus)
		if e != nil || v.Transcoders != c.want {
			t.Fatal(c, v, e)
		}
	}
	for _, bad := range []string{"-1", "129", "wrong", "0"} {
		if _, e := load(func(k string) string {
			if k == "KORE_EDGE_TRANSCODERS" {
				return bad
			}
			return ""
		}, 32); e == nil {
			t.Fatal("invalid concurrency", bad)
		}
	}
	v, e := load(func(k string) string {
		switch k {
		case "KORE_EDGE_TRANSCODERS":
			return "12"
		case "KORE_EDGE_ENCODER_THREADS":
			return "0"
		}
		return ""
	}, 32)
	if e != nil || v.Transcoders != 12 || v.EncoderThreads != 0 {
		t.Fatal(v, e)
	}
	for _, bad := range []string{"-1", "65", "wrong"} {
		if _, e := load(func(k string) string {
			if k == "KORE_EDGE_ENCODER_THREADS" {
				return bad
			}
			return ""
		}, 32); e == nil {
			t.Fatal("invalid workers", bad)
		}
	}
}

func TestExplicitCapacity(t *testing.T) {
	defaults, e := load(func(string) string { return "" }, 8)
	if e != nil || defaults != (Limits{16, 2, 2, 2, 0}) {
		t.Fatal(defaults, e)
	}
	values := map[string]string{"KORE_EDGE_MAX_CHANNELS": "128", "KORE_EDGE_HLS_INPUTS": "40", "KORE_EDGE_HLS_OUTPUTS": "16"}
	limits, e := load(func(k string) string { return values[k] }, 8)
	if e != nil || limits != (Limits{128, 40, 16, 2, 0}) {
		t.Fatal(limits, e)
	}
	for _, bad := range []string{"0", "-1", "513", "invalid"} {
		values["KORE_EDGE_MAX_CHANNELS"] = bad
		if _, e := load(func(k string) string { return values[k] }, 8); e == nil {
			t.Fatal("invalid capacity accepted", bad)
		}
	}
}
