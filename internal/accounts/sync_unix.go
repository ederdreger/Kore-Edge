//go:build !windows

package accounts

import "os"

func syncDirectory(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return ErrStorage
	}
	defer f.Close()
	if f.Sync() != nil {
		return ErrStorage
	}
	return nil
}
