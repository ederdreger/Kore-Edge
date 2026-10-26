//go:build !linux

package multicast

import "errors"

func openSocket(Target, int) (Socket, error) {
	return nil, errors.New("multicast transmission requires Linux")
}
