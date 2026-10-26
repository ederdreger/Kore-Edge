package wireguard

import (
	"os"
	"syscall"
)

func credentialLock(path string) (*os.File, error) {
	if i, e := os.Lstat(path); e == nil && !i.Mode().IsRegular() {
		return nil, ErrCredentials
	}
	p, e := syscall.UTF16PtrFromString(path)
	if e != nil {
		return nil, e
	}
	h, e := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		return nil, e
	}
	return os.NewFile(uintptr(h), path), nil
}
