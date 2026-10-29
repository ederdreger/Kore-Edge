//go:build !linux

package multicast

import "errors"

func linuxCompatibleInterfaces() ([]Interface, error) {
	return nil, errors.New("interface inventory unavailable")
}
