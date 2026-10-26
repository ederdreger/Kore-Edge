package security

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

func privateDirectory(path string, create bool) error {
	if !filepath.IsAbs(path) || filepath.Dir(filepath.Clean(path)) == filepath.Clean(path) {
		return ErrUnavailable
	}
	cwd, e := os.Getwd()
	if e != nil {
		return ErrUnavailable
	}
	rel, e := filepath.Rel(cwd, path)
	if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrUnavailable
	}
	if create {
		if os.MkdirAll(path, 0700) != nil {
			return ErrUnavailable
		}
	}
	info, e := os.Lstat(path)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privateMode(info) != nil {
		return ErrUnavailable
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return ErrUnavailable
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	return nil
}
func readPrivate(path string, limit int64) ([]byte, error) {
	if privateDirectory(filepath.Dir(path), false) != nil {
		return nil, ErrUnavailable
	}
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || privateMode(info) != nil || info.Size() > limit {
		return nil, ErrUnavailable
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || len(b) > int(limit) {
		return nil, ErrUnavailable
	}
	return b, nil
}
