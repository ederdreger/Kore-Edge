package security

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
)

// OpenAudit creates a private, bounded audit writer for a local control module.
func OpenAudit(directory string) (*Audit, error) { return openAudit(directory) }

// PrivateDirectory validates a private runtime directory outside the working
// tree. Callers must run from the checkout root when protecting the checkout.
func PrivateDirectory(path string, create bool) error { return privateDirectory(path, create) }

// ReadPrivateFile reads only a bounded regular file in a private directory.
func ReadPrivateFile(path string, limit int64) ([]byte, error) { return readPrivate(path, limit) }

// PrivateFileSHA256 hashes a bounded private regular file without buffering its
// contents. A second identity check rejects replacement during open.
func PrivateFileSHA256(path string, limit int64) (string, error) {
	if privateDirectory(filepath.Dir(path), false) != nil {
		return "", ErrUnavailable
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || privateMode(info) != nil || info.Size() > limit {
		return "", ErrUnavailable
	}
	f, e := os.Open(path)
	if e != nil {
		return "", ErrUnavailable
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return "", ErrUnavailable
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, limit+1))
	if e != nil || n > limit || n == 0 {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
