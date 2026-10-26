//go:build !windows

package wireguard

import "os"

func credentialSync(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return ErrCredentials
	}
	defer f.Close()
	if f.Sync() != nil {
		return ErrCredentials
	}
	return nil
}
