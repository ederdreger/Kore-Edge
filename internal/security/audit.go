package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const MaxAuditBytes = 8 * 1024 * 1024
const maxEventBytes = 512

type Event struct {
	Time        time.Time `json:"time"`
	RequestID   string    `json:"request_id"`
	PrincipalID string    `json:"principal_id"`
	Role        string    `json:"role"`
	Route       string    `json:"route"`
	Method      string    `json:"method"`
	Phase       string    `json:"phase"`
	Status      int       `json:"status"`
}
type Audit struct {
	mu             sync.Mutex
	file           *os.File
	lock           string
	size, reserved int64
	closed         bool
	broken         bool
}

func openAudit(directory string) (*Audit, error) {
	if privateDirectory(directory, true) != nil {
		return nil, ErrUnavailable
	}
	lock := filepath.Join(directory, "audit.lock")
	f, e := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, ErrUnavailable
	}
	f.Close()
	ok := false
	defer func() {
		if !ok {
			os.Remove(lock)
		}
	}()
	path := filepath.Join(directory, "audit.jsonl")
	info, e := os.Lstat(path)
	if e != nil && !os.IsNotExist(e) {
		return nil, ErrUnavailable
	}
	if e == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || privateMode(info) != nil || info.Size() > MaxAuditBytes) {
		return nil, ErrUnavailable
	}
	file, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, ErrUnavailable
	}
	info, e = file.Stat()
	if e != nil {
		file.Close()
		return nil, ErrUnavailable
	}
	ok = true
	return &Audit{file: file, lock: lock, size: info.Size()}, nil
}
func (a *Audit) append(event Event) error {
	b, e := json.Marshal(event)
	if e != nil || len(b)+1 > maxEventBytes {
		return ErrUnavailable
	}
	b = append(b, '\n')
	if a.closed || a.broken || a.size+a.reserved+int64(len(b)) > MaxAuditBytes {
		return ErrUnavailable
	}
	if n, err := a.file.Write(b); err != nil || n != len(b) {
		a.broken = true
		return ErrUnavailable
	}
	a.size += int64(len(b))
	if a.file.Sync() != nil {
		a.broken = true
		return ErrUnavailable
	}
	return nil
}
func (a *Audit) Begin(event Event) (func(int) error, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.broken || a.size+a.reserved+2*maxEventBytes > MaxAuditBytes {
		return nil, ErrUnavailable
	}
	event.Phase = "authorized"
	if a.append(event) != nil {
		return nil, ErrUnavailable
	}
	a.reserved += maxEventBytes
	return func(status int) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.reserved -= maxEventBytes
		event.Time = time.Now().UTC()
		event.Phase = "completed"
		event.Status = status
		return a.append(event)
	}, nil
}
func (a *Audit) Deny(event Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	event.Phase = "denied"
	return a.append(event)
}
func (a *Audit) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil
	}
	a.closed = true
	e := a.file.Close()
	_ = os.Remove(a.lock)
	return e
}
