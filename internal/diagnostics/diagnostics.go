// Package diagnostics provides read-only host observations, never host control.
package diagnostics

import "time"

type Metric struct {
	Timestamp time.Time `json:"timestamp"`
	State     string    `json:"state"`
	Source    string    `json:"source"`
	Value     any       `json:"value"`
	Reason    string    `json:"reason,omitempty"`
}

type Snapshot struct {
	Timestamp  time.Time `json:"timestamp"`
	CPU        Metric    `json:"cpu"`
	Memory     Metric    `json:"memory"`
	Disk       Metric    `json:"disk"`
	Interfaces Metric    `json:"interfaces"`
	Routes     Metric    `json:"routes"`
	Sensors    Metric    `json:"sensors"`
}

func unavailable(now time.Time, source, reason string) Metric {
	return Metric{Timestamp: now, State: "unavailable", Source: source, Reason: reason}
}

func observed(now time.Time, source string, value any, err error) Metric {
	if err != nil {
		return unavailable(now, source, "source missing, unreadable or invalid")
	}
	return Metric{Timestamp: now, State: "real", Source: source, Value: value}
}
