package multicast

import "context"

// OpenAuthorized uses the same explicit-interface socket and validation as the
// laboratory sender. It never grants capabilities or changes network settings.
func OpenAuthorized(ctx context.Context, target Target, interfaces []Interface) (*Sender, error) {
	return New(ctx, target, interfaces, openSocket)
}
