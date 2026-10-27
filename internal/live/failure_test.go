package live

import (
	"errors"
	"io"
	"kore-edge/internal/edgecatalog"
	"testing"
)

func TestFailureReasonsDoNotExposeSourceDetails(t *testing.T) {
	for _, entry := range []struct {
		err    error
		reason string
	}{
		{&edgecatalog.RemoteStatusError{Code: 429}, "matrix_http_429"}, {errors.Join(errSourceOpen, edgecatalog.ErrResponseTimeout), "matrix_response_timeout"}, {errSourceIdle, "source_idle_timeout"}, {errors.Join(errSourceOpen, errors.New("https://synthetic.invalid/private-token")), "source_open_failed"}, {ErrInvalid, "source_invalid_transport"}, {io.EOF, "source_closed"}, {io.ErrUnexpectedEOF, "source_closed"}, {errUDPOutput, "udp_output_failed"}, {errHLSOutput, "hls_output_failed"}, {errors.New("synthetic secret"), "source_read_failed"},
	} {
		if got := failureReason(entry.err); got != entry.reason {
			t.Fatal(got, entry.reason)
		}
	}
}
