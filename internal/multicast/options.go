package multicast

// Socket options are injectable so tests never open a network socket.
type options interface {
	Device(string) error
	Interface(int, string) error
	TTL(int) error
	Loopback(bool) error
	Buffer(int) error
}

func applyOptions(t Target, index int, o options) error {
	if e := o.Device(t.Interface); e != nil {
		return e
	}
	if e := o.Interface(index, t.SourceIP); e != nil {
		return e
	}
	if e := o.TTL(t.TTL); e != nil {
		return e
	}
	if e := o.Loopback(false); e != nil {
		return e
	}
	return o.Buffer(64 * 1024)
}
