package monitoring

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

// Prometheus 0.0.4 text exposition. Unavailable observations have availability
// zero but no numeric sample. These local counters may reset on channel retry.
func Export(w io.Writer, s Snapshot) {
	measurements := append([]Measurement(nil), s.Measurements...)
	sort.Slice(measurements, func(i, j int) bool {
		a, b := measurements[i], measurements[j]
		return a.Name+a.ChannelID+a.Interface < b.Name+b.ChannelID+b.Interface
	})
	families := map[string][]string{}
	types := map[string]string{}
	add := func(name, kind, labels string, value float64) {
		types[name] = kind
		families[name] = append(families[name], fmt.Sprintf("%s{%s} %g\n", name, labels, value))
	}
	samplerUp := 0.0
	if s.State == "measured" {
		samplerUp = 1
	}
	add("kore_edge_monitoring_sample_available", "gauge", `source="local_sampler",origin="process"`, samplerUp)
	for _, m := range measurements {
		name := "kore_edge_" + m.Name
		labels := fmt.Sprintf("channel_id=%s,interface=%s,source=%s,origin=%s", quote(m.ChannelID), quote(m.Interface), quote(m.Source), quote(m.Origin))
		available := 0
		if m.State == "measured" && m.Value != nil && !math.IsNaN(*m.Value) && !math.IsInf(*m.Value, 0) {
			available = 1
		}
		add(name+"_available", "gauge", labels, float64(available))
		if available == 0 {
			continue
		}
		kind := "gauge"
		if strings.HasSuffix(name, "_total") {
			kind = "counter"
		}
		add(name, kind, labels, *m.Value)
		if !m.Timestamp.IsZero() {
			add(name+"_timestamp_seconds", "gauge", labels, float64(m.Timestamp.UnixNano())/float64(time.Second))
		}
		window := m.WindowEnd.Sub(m.WindowStart).Seconds()
		if !m.WindowStart.IsZero() && window >= 0 {
			add(name+"_window_seconds", "gauge", labels, window)
		}
	}
	for _, a := range s.Alerts {
		value := 0
		if a.State == "firing" {
			value = 1
		}
		labels := fmt.Sprintf("code=%s,channel_id=%s,severity=%s,state=%s", quote(a.Code), quote(a.ChannelID), quote(a.Severity), quote(a.State))
		add("kore_edge_alert_active", "gauge", labels, float64(value))
	}
	names := make([]string, 0, len(families))
	for name := range families {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(w, "# HELP %s Kore Edge observation; source, origin and window exported explicitly.\n# TYPE %s %s\n", name, name, types[name])
		for _, line := range families[name] {
			_, _ = io.WriteString(w, line)
		}
	}
}
func quote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}
