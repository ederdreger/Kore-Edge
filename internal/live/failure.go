package live

import (
	"encoding/json"
	"errors"
	"io"
	"kore-edge/internal/edgecatalog"
	"log"
	"strconv"
	"time"
)

var errSourceIdle = errors.New("source_idle_timeout")
var errSourceOpen = errors.New("source_open_failed")
var errUDPOutput = errors.New("udp_output_failed")
var errHLSOutput = errors.New("hls_output_failed")

func failureReason(err error) string {
	var remoteStatus *edgecatalog.RemoteStatusError
	if errors.As(err, &remoteStatus) && remoteStatus.Code >= 400 && remoteStatus.Code <= 599 {
		return "matrix_http_" + strconv.Itoa(remoteStatus.Code)
	}
	switch {
	case errors.Is(err, edgecatalog.ErrResponseTimeout):
		return "matrix_response_timeout"
	case errors.Is(err, errSourceIdle):
		return "source_idle_timeout"
	case errors.Is(err, errSourceOpen):
		return "source_open_failed"
	case errors.Is(err, errUDPOutput):
		return "udp_output_failed"
	case errors.Is(err, errHLSOutput):
		return "hls_output_failed"
	case errors.Is(err, ErrInvalid):
		return "source_invalid_transport"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "source_closed"
	default:
		return "source_read_failed"
	}
}

// Aggregate diagnostics omit channel identities, URLs, error strings and secrets.
// A separate mutex avoids blocking shutdown while the manager waits for a job.
func (m *Manager) recordFailure(protocol, reason string) {
	m.failureMu.Lock()
	defer m.failureMu.Unlock()
	if m.failureCounts == nil {
		m.failureCounts = map[string]int{}
	}
	m.failureCounts[protocol+":"+reason]++
	if time.Since(m.failureLoggedAt) < 30*time.Second {
		return
	}
	data, _ := json.Marshal(m.failureCounts)
	log.Printf("live_reception_failures counts=%s", data)
	m.failureCounts = map[string]int{}
	m.failureLoggedAt = time.Now()
}
