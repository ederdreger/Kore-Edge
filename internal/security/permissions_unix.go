//go:build !windows

package security

import "os"

func privateMode(info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return ErrUnavailable
	}
	return nil
}
func permissionsVerified() bool { return true }
