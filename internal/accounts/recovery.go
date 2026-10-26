package accounts

import "golang.org/x/crypto/bcrypt"

// ResetHash is an offline recovery operation protected by the store's exclusive
// kernel lock. It never creates accounts or bypasses the active-admin invariant.
func (s *Store) ResetHash(username, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cost, e := bcrypt.Cost([]byte(hash))
	if s.closed || e != nil || cost < 10 || cost > 14 {
		return ErrRejected
	}
	next := database{Version: 1, Users: append([]record(nil), s.db.Users...)}
	for i, u := range next.Users {
		if u.Username == username && u.Role == "admin" && !u.Disabled {
			next.Users[i].Hash = hash
			next.Users[i].Revision++
			return s.save(next, "recovery", "admin", "account_recovery")
		}
	}
	return ErrRejected
}
