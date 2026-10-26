//go:build windows

package operations

// Go cannot fsync a directory on Windows. File contents are synced; crash
// durability of rename must be validated on the deployment filesystem.
func syncDirectory(string) error { return nil }
