package accounts

import (
	"context"
	"golang.org/x/crypto/bcrypt"
	"kore-edge/internal/security"
	"time"
)

// UserFromContext is populated only after session and CSRF verification.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(accountKey{}).(User)
	return u, ok
}

// ChannelChange uses the already verified panel session for routine channel
// operations. It grants no access to host administration or account changes.
// Wrap has checked Origin, session expiry and CSRF before populating actor.
func (s *Store) ChannelChange(actor User, action string, change func() error) (result error) {
	if !s.channelMu.TryLock() {
		return ErrBusy
	}
	defer s.channelMu.Unlock()
	s.mu.Lock()
	if s.closed || actor.Role != "admin" || (action != "manual_channel_save" && action != "manual_channel_metadata" && action != "manual_channel_delete" && action != "manual_channel_probe" && action != "live_config" && action != "live_start" && action != "live_stop") {
		s.mu.Unlock()
		return ErrRejected
	}
	valid := false
	for _, u := range s.db.Users {
		if u.ID == actor.ID && u.Revision == actor.Revision && !u.Disabled && u.Role == "admin" {
			valid = true
		}
	}
	if !valid {
		s.mu.Unlock()
		return ErrRejected
	}
	nonce := token()
	if len(nonce) < 16 {
		s.mu.Unlock()
		return ErrStorage
	}
	end, e := s.audit.Begin(security.Event{Time: time.Now().UTC(), RequestID: nonce[:16], PrincipalID: actor.ID, Role: "admin", Route: action, Method: "POST"})
	s.mu.Unlock()
	if e != nil {
		return ErrStorage
	}
	status := 500
	defer func() {
		if end(status) != nil {
			result = ErrStorage
		}
	}()
	result = change()
	if result == nil {
		status = 200
	}
	return result
}

// WireGuardChange restricts a small set of local preparation operations. It does
// not grant the session general access to operational APIs or host commands.
func (s *Store) WireGuardChange(actor User, password, action string, change func() error) (result error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || actor.Role != "admin" || len(password) > 72 || (action != "wireguard_identity" && action != "wireguard_peer" && action != "wireguard_connection" && action != "wireguard_pairing" && action != "partner_catalog_config" && action != "partner_catalog_sync" && action != "live_config" && action != "live_start" && action != "live_stop") {
		return ErrRejected
	}
	now := time.Now().UTC()
	if now.Sub(s.window) > time.Minute {
		s.failures = map[string]int{}
		s.window = now
	}
	name := "wg:" + actor.ID
	if s.failures[name] >= 5 || s.failures["*"] >= 30 {
		return ErrRejected
	}
	s.failures[name]++
	s.failures["*"]++
	index := -1
	for i, u := range s.db.Users {
		if u.ID == actor.ID && u.Revision == actor.Revision && !u.Disabled && u.Role == "admin" {
			index = i
		}
	}
	if index < 0 || bcrypt.CompareHashAndPassword([]byte(s.db.Users[index].Hash), []byte(password)) != nil {
		return ErrRejected
	}
	delete(s.failures, name)
	nonce := token()
	if len(nonce) < 16 {
		return ErrStorage
	}
	end, e := s.audit.Begin(security.Event{Time: now, RequestID: nonce[:16], PrincipalID: actor.ID, Role: "admin", Route: action, Method: "POST"})
	if e != nil {
		return ErrStorage
	}
	status := 500
	defer func() {
		if end(status) != nil {
			result = ErrStorage
		}
	}()
	result = change()
	if result == nil {
		status = 200
	}
	return result
}

// PlaybackOwner validates a capability owner without granting a panel session.
func (s *Store) PlaybackOwner(id string, revision uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	for _, u := range s.db.Users {
		if u.ID == id && u.Revision == revision && !u.Disabled && u.Role == "admin" {
			return true
		}
	}
	return false
}
