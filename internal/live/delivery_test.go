package live

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type efficientWriter struct {
	*httptest.ResponseRecorder
	copied   bool
	deadline bool
}

func (w *efficientWriter) ReadFrom(r io.Reader) (int64, error) {
	w.copied = true
	return io.Copy(w.ResponseRecorder, r)
}
func (w *efficientWriter) SetWriteDeadline(time.Time) error { w.deadline = true; return nil }
func TestDeliveryCountsHTTPPayloadAndConnectionsWithoutChangingData(t *testing.T) {
	m := &Manager{}
	underlying := &efficientWriter{ResponseRecorder: httptest.NewRecorder()}
	w, finish := m.observeDelivery(underlying, "synthetic", true)
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now()); err != nil || !underlying.deadline {
		t.Fatal("response controller no longer unwraps")
	}
	payload := []byte("synthetic TS payload")
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	stats := m.deliverySnapshot("synthetic")
	if stats.Bytes != uint64(len(payload)) || stats.Clients != 1 || stats.Rate != nil || !bytes.Equal(payload, underlying.Body.Bytes()) {
		t.Fatal("incorrect HTTP delivery accounting", stats)
	}
	finish()
	finish()
	if m.deliverySnapshot("synthetic").Clients != 0 {
		t.Fatal("connection count leaked or became negative")
	}
	w, done := m.observeDelivery(underlying, "synthetic", false)
	defer done()
	if _, err := w.(io.ReaderFrom).ReadFrom(bytes.NewBufferString("HLS")); err != nil || !underlying.copied {
		t.Fatal("efficient reader forwarding lost")
	}
	if stats = m.deliverySnapshot("synthetic"); stats.Clients != 0 || stats.Bytes != uint64(len(payload)+3) {
		t.Fatal("HLS counted as a TS connection or payload duplicated")
	}
}
