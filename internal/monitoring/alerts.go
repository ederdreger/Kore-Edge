package monitoring

import "time"

type Alert struct {
	ID        string    `json:"id"`
	Code      string    `json:"code"`
	ChannelID string    `json:"channel_id"`
	Severity  string    `json:"severity"`
	State     string    `json:"state"`
	Since     time.Time `json:"since"`
	UpdatedAt time.Time `json:"updated_at"`
}
type condition struct {
	code, channel, severity string
	active                  bool
	hold                    time.Duration
}

func (s *Service) evaluate(conditions []condition, now time.Time) {
	seen := map[string]bool{}
	for _, c := range conditions {
		id := c.code + ":" + c.channel
		seen[id] = true
		old, exists := s.active[id]
		if !c.active {
			if exists {
				s.resolve(id, now)
			}
			continue
		}
		if !exists {
			old = Alert{ID: id, Code: c.code, ChannelID: c.channel, Severity: c.severity, State: "pending", Since: now, UpdatedAt: now}
		}
		if now.Sub(old.Since) >= c.hold && old.State != "firing" {
			old.State = "firing"
			old.UpdatedAt = now
		}
		s.active[id] = old
	}
	for id := range s.active {
		if !seen[id] {
			s.resolve(id, now)
		}
	}
}
func (s *Service) resolve(id string, now time.Time) {
	alert := s.active[id]
	delete(s.active, id)
	if alert.State != "firing" {
		return
	}
	alert.State = "resolved"
	alert.UpdatedAt = now
	s.recent = append(s.recent, alert)
	if len(s.recent) > 128 {
		s.recent = append([]Alert(nil), s.recent[len(s.recent)-128:]...)
	}
}
