package live

import (
	"context"
	"io"
	"kore-edge/internal/mediaresources"
	"path/filepath"
	"testing"
)

func TestMediaSaturationQueuesWithoutCountingSourceFailure(t *testing.T) {
	c := &syntheticCatalog{}
	c.allowed.Store(true)
	m, e := New(context.Background(), filepath.Join(t.TempDir(), "live"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	m.open = func(context.Context, ChannelConfig, Tools) (io.ReadCloser, error) { return nil, mediaresources.ErrBusy }
	if e = m.Save(ChannelConfig{ChannelID: "synthetic-resource", Protocol: "http_ts"}, Tools{}, 0); e != nil {
		t.Fatal(e)
	}
	if e = m.Start("synthetic-resource"); e != nil {
		t.Fatal(e)
	}
	waitStatus(t, m, "waiting_hls_capacity")
	status := m.Snapshot().Streams[0]
	if status.Interruptions != 0 || status.Error != "media_resources_busy" {
		t.Fatal("resource wait reported as signal drop", status)
	}
	if e = m.Stop("synthetic-resource"); e != nil {
		t.Fatal(e)
	}
	if m.IsActive("synthetic-resource") {
		t.Fatal("queued source did not stop")
	}
}

func TestHLSValidationReservationIsReleasedOnDenial(t *testing.T) {
	first, ok := mediaresources.Output()
	if !ok {
		t.Fatal("first slot")
	}
	defer first()
	second, ok := mediaresources.Output()
	if !ok {
		t.Fatal("second slot")
	}
	defer second()
	c := &syntheticCatalog{}
	c.allowed.Store(true)
	m, e := New(context.Background(), filepath.Join(t.TempDir(), "live"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	j := &job{config: ChannelConfig{ChannelID: "synthetic-resource", Protocol: "http_ts"}}
	m.prepareHLS(context.Background(), j, nil)
	if j.status.HLS != "waiting_resources" {
		t.Fatal("missing HLS queue", j.status.HLS)
	}
	first()
	m.prepareHLS(context.Background(), j, nil)
	j.hlsWG.Wait()
	if j.status.HLS != "unauthorized" {
		t.Fatal("permission bypassed")
	}
	release, ok := mediaresources.Output()
	if !ok {
		t.Fatal("failed validation leaked a process reservation")
	}
	release()
}
