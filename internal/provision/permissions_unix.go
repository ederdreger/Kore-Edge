//go:build !windows

package provision

import "os"

func privatePermissions(info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return ErrStorage
	}
	return nil
}
func permissionsVerified() bool { return true }
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
