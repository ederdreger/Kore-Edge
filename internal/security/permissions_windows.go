//go:build windows

package security

import "os"

func privateMode(os.FileInfo) error { return nil }
func permissionsVerified() bool     { return false } // POSIX mode bits do not certify Windows ACLs.
