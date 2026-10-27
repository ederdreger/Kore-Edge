package live

import (
	"strings"
	"time"
)

// Automatic reception is independent of browser sessions. Manual stops are
// persisted; failed jobs keep their bounded retry budget until explicitly started.
func (m *Manager) RunAutomatic() {
	m.autoOnce.Do(func() {
		m.autoWG.Add(1)
		go func() {
			defer m.autoWG.Done()
			timer := time.NewTicker(5 * time.Second)
			defer timer.Stop()
			for {
				m.reconcileAutomatic()
				select {
				case <-m.ctx.Done():
					return
				case <-timer.C:
				}
			}
		}()
	})
}

func (m *Manager) defaultChannel(id string) (ChannelConfig, error) {
	snapshot := m.catalog.Snapshot()
	if !snapshot.Available || snapshot.Catalog == nil {
		return ChannelConfig{}, ErrDenied
	}
	for _, c := range snapshot.Catalog.Channels {
		if c.ID != id {
			continue
		}
		// Preserve an existing selection; this preference applies only to new inputs.
		for _, protocol := range []string{"http_ts", "srt", "hls", "udp"} {
			for _, p := range c.Protocols {
				if p.ID == protocol && m.catalog.Authorized(id, protocol) {
					return ChannelConfig{ChannelID: id, Protocol: protocol, Resume: true}, nil
				}
			}
		}
	}
	return ChannelConfig{}, ErrDenied
}

// Caller holds m.mu. New inputs never enable UDP/HLS outputs implicitly.
func (m *Manager) prepareStartLocked(id string) error {
	if m.closed || m.ctx.Err() != nil {
		return ErrUnavailable
	}
	current := m.store.snapshot()
	found := false
	for i, c := range current.Channels {
		if c.ChannelID == id {
			if !m.catalog.Authorized(id, c.Protocol) {
				return ErrDenied
			}
			current.Channels[i].AutoPaused = false
			current.Channels[i].Resume = true
			found = true
			break
		}
	}
	if !found {
		c, e := m.defaultChannel(id)
		if e != nil {
			return e
		}
		current.Channels = append(current.Channels, c)
	}
	if current.Tools.FFmpeg == "" {
		cap, _ := m.Capabilities()
		current.Tools = cap.Tools
	}
	for _, c := range current.Channels {
		if c.ChannelID == id && c.Protocol == "hls" && current.Tools.FFmpeg == "" {
			return ErrUnavailable
		}
	}
	if e := m.store.replace(current, current.Generation); e != nil {
		return e
	}
	return m.startLocked(id)
}

func (m *Manager) reconcileAutomatic() {
	snapshot := m.catalog.Snapshot()
	if !snapshot.Available || snapshot.Catalog == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	for _, c := range snapshot.Catalog.Channels {
		if strings.HasPrefix(c.ID, "local_") {
			continue
		}
		// No retry loop for terminal failures; no restart of a manually stopped job.
		if m.jobs[c.ID] != nil {
			continue
		}
		paused := false
		for _, entry := range m.store.snapshot().Channels {
			if entry.ChannelID == c.ID {
				paused = entry.AutoPaused
				// Previously prepared outputs without permission to resume are
				// not activated by introducing automatic input reception.
				if !entry.Resume && (entry.UDP != nil || entry.HLS) {
					paused = true
				}
				break
			}
		}
		if paused {
			continue
		}
		// Admission is bounded before allocating configs or contacting sources.
		active := 0
		for _, j := range m.jobs {
			select {
			case <-j.done:
			default:
				active++
			}
		}
		if active >= 16 {
			// Queue a new input for a later free slot, without starting a source.
			current := m.store.snapshot()
			found := false
			for _, entry := range current.Channels {
				if entry.ChannelID == c.ID {
					found = true
					break
				}
			}
			if !found && len(current.Channels) < 512 {
				if entry, e := m.defaultChannel(c.ID); e == nil {
					current.Channels = append(current.Channels, entry)
					_ = m.store.replace(current, current.Generation)
				}
			}
			continue
		}
		_ = m.prepareStartLocked(c.ID)
	}
}
