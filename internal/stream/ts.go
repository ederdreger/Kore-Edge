package stream

import (
	"errors"
	"io"
)

const PacketSize = 188

var errTS = errors.New("invalid transport stream")

// ingest accepts aligned 188-byte MPEG-TS packets. Corrupt/truncated framing
// fails the attempt instead of silently inventing packets or resynchronizing.
func ingest(r io.Reader, observe func(bytes, packets, syncErrors, tei uint64)) error {
	return ingestTo(r, observe, nil)
}
func ingestTo(r io.Reader, observe func(bytes, packets, syncErrors, tei uint64), send func([]byte) error) error {
	var packet [PacketSize]byte
	for {
		n, e := io.ReadFull(r, packet[:])
		if e != nil {
			if e == io.EOF {
				return nil
			}
			if e == io.ErrUnexpectedEOF {
				observe(uint64(n), 0, 1, 0)
				return errTS
			}
			observe(uint64(n), 0, 0, 0)
			return e
		}
		control := (packet[3] >> 4) & 3
		if packet[0] != 0x47 || control == 0 || (control == 2 && packet[4] != 183) || (control == 3 && packet[4] > 182) {
			observe(uint64(n), 0, 1, 0)
			return errTS
		}
		errors := uint64(0)
		if packet[1]&0x80 != 0 {
			errors = 1
		}
		observe(uint64(n), 1, 0, errors)
		if send != nil {
			if err := send(packet[:]); err != nil {
				return err
			}
		}
	}
}
