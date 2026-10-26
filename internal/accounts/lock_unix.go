//go:build !windows

package accounts

import (
	"os"
	"syscall"
)

func acquireLock(path string) (*os.File, error) {
	if i, e := os.Lstat(path); e == nil && (!i.Mode().IsRegular() || i.Mode().Perm()&0077 != 0) {
		return nil, ErrStorage
	}
	f, e := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
