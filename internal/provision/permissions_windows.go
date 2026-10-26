//go:build windows

package provision

import "os"

// Windows mode bits do not prove ACL isolation. Require operator ACL review;
// never claim that os.Chmod(0600) established a private Windows DACL.
func privatePermissions(os.FileInfo) error { return nil }
func permissionsVerified() bool            { return false }
func syncDirectory(string) error           { return nil }
