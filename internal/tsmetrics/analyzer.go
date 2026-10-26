// Package tsmetrics observes transport headers; it does not alter stream data.
package tsmetrics

import (
	"bytes"
	"time"
)

const PCRModulus = uint64(1<<33) * 300

type Snapshot struct {
	WindowStart        time.Time  `json:"window_start"`
	WindowEnd          time.Time  `json:"window_end"`
	CCState            string     `json:"cc_state"`
	ContinuityPackets  uint64     `json:"continuity_packets"`
	CCErrors           uint64     `json:"cc_errors"`
	DuplicatePackets   uint64     `json:"duplicate_packets"`
	TEI                uint64     `json:"tei"`
	Malformed          uint64     `json:"malformed"`
	Discontinuities    uint64     `json:"discontinuities"`
	PCRState           string     `json:"pcr_state"`
	PCRCount           uint64     `json:"pcr_count"`
	PCRWraps           uint64     `json:"pcr_wraps"`
	PCRBackwards       uint64     `json:"pcr_backwards"`
	PCRInvalid         uint64     `json:"pcr_invalid"`
	PCRPID             *uint16    `json:"pcr_pid"`
	PCRIntervalSeconds *float64   `json:"pcr_interval_seconds"`
	PCRObservedAt      *time.Time `json:"pcr_observed_at"`
	PCRPairStart       *time.Time `json:"pcr_pair_start"`
}

func (s Snapshot) Clone() Snapshot {
	if s.PCRPairStart != nil {
		v := *s.PCRPairStart
		s.PCRPairStart = &v
	}
	if s.PCRPID != nil {
		v := *s.PCRPID
		s.PCRPID = &v
	}
	if s.PCRIntervalSeconds != nil {
		v := *s.PCRIntervalSeconds
		s.PCRIntervalSeconds = &v
	}
	if s.PCRObservedAt != nil {
		v := *s.PCRObservedAt
		s.PCRObservedAt = &v
	}
	return s
}

type pidState struct {
	at        time.Time
	valid     bool
	cc        byte
	last      [188]byte
	duplicate bool
	hasPCR    bool
	pcr       uint64
}
type Analyzer struct {
	pids  [8192]*pidState
	stats Snapshot
}

func New(now time.Time) *Analyzer {
	return &Analyzer{stats: Snapshot{WindowStart: now, WindowEnd: now, CCState: "unavailable", PCRState: "unavailable"}}
}
func (a *Analyzer) Snapshot() Snapshot { return a.stats.Clone() }
func (a *Analyzer) Observe(p []byte, now time.Time) {
	a.stats.WindowEnd = now
	if len(p) != 188 || p[0] != 0x47 {
		a.stats.Malformed++
		return
	}
	pid := uint16(p[1]&31)<<8 | uint16(p[2])
	control := (p[3] >> 4) & 3
	if control == 0 || (control == 2 && p[4] != 183) || (control == 3 && p[4] > 182) {
		a.stats.Malformed++
		return
	}
	if p[1]&0x80 != 0 {
		a.stats.TEI++
		if state := a.pids[pid]; state != nil {
			state.valid = false
			state.hasPCR = false
		}
		return
	}
	if pid == 8191 {
		return
	} // Null packets have no useful continuity or PCR guarantee.
	state := a.pids[pid]
	if state == nil {
		state = &pidState{}
		a.pids[pid] = state
	}
	discontinuity := control >= 2 && p[4] > 0 && p[5]&0x80 != 0
	if discontinuity {
		a.stats.Discontinuities++
		state.valid = false
		state.hasPCR = false
		if a.stats.PCRPID != nil && *a.stats.PCRPID == pid {
			a.stats.PCRIntervalSeconds = nil
			a.stats.PCRPairStart = nil
		}
	}
	cc := p[3] & 15
	payload := control&1 != 0
	a.stats.CCState = "measured"
	a.stats.ContinuityPackets++
	duplicate := false
	repeat := state.valid && payload && cc == state.cc
	if state.valid {
		expected := state.cc
		if payload {
			expected = (expected + 1) & 15
		}
		if payload && cc == state.cc && bytes.Equal(p, state.last[:]) && !state.duplicate {
			a.stats.DuplicatePackets++
			duplicate = true
		} else if cc != expected {
			a.stats.CCErrors++
		}
	}
	state.valid = true
	state.cc = cc
	state.duplicate = repeat
	copy(state.last[:], p)
	if duplicate {
		return
	}
	if control < 2 || p[4] == 0 || p[5]&0x10 == 0 {
		return
	}
	if p[4] < 7 {
		a.stats.PCRInvalid++
		return
	}
	b := p[6:12]
	base := uint64(b[0])<<25 | uint64(b[1])<<17 | uint64(b[2])<<9 | uint64(b[3])<<1 | uint64(b[4]>>7)
	ext := uint64(b[4]&1)<<8 | uint64(b[5])
	if ext >= 300 || b[4]&0x7e != 0x7e {
		a.stats.PCRInvalid++
		return
	}
	ticks := base*300 + ext
	a.stats.PCRCount++
	a.stats.PCRState = "observed"
	a.stats.PCRPID = &pid
	stamp := now
	a.stats.PCRObservedAt = &stamp
	a.stats.PCRIntervalSeconds = nil
	a.stats.PCRPairStart = nil
	if state.hasPCR {
		var delta uint64
		valid := true
		if ticks >= state.pcr {
			delta = ticks - state.pcr
		} else if state.pcr-ticks > PCRModulus/2 {
			delta = PCRModulus - state.pcr + ticks
			a.stats.PCRWraps++
		} else {
			a.stats.PCRBackwards++
			valid = false
		}
		if valid {
			interval := float64(delta) / 27000000
			a.stats.PCRIntervalSeconds = &interval
			stamp := state.at
			a.stats.PCRPairStart = &stamp
		}
	}
	state.pcr = ticks
	state.hasPCR = true
	state.at = now
}
