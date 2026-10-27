package capacity

import "testing"

func TestExplicitCapacity(t *testing.T) {
	defaults, e := Load(func(string) string { return "" })
	if e != nil || defaults != (Limits{16, 2, 2}) {
		t.Fatal(defaults, e)
	}
	values := map[string]string{"KORE_EDGE_MAX_CHANNELS": "128", "KORE_EDGE_HLS_INPUTS": "40", "KORE_EDGE_HLS_OUTPUTS": "16"}
	limits, e := Load(func(k string) string { return values[k] })
	if e != nil || limits != (Limits{128, 40, 16}) {
		t.Fatal(limits, e)
	}
	for _, bad := range []string{"0", "-1", "513", "invalid"} {
		values["KORE_EDGE_MAX_CHANNELS"] = bad
		if _, e := Load(func(k string) string { return values[k] }); e == nil {
			t.Fatal("invalid capacity accepted", bad)
		}
	}
}
