package live

import (
	"context"
	"fmt"
	"kore-edge/internal/edgecatalog"
	"path/filepath"
	"testing"
)

type automaticCatalog struct {
	syntheticCatalog
	channels []edgecatalog.Channel
}

func (c *automaticCatalog) Snapshot() edgecatalog.Snapshot {
	return edgecatalog.Snapshot{Available: c.allowed.Load(), Catalog: &edgecatalog.Manifest{Channels: c.channels}}
}
func automaticManager(t *testing.T, n int) (*Manager, *automaticCatalog, string) {
	t.Helper()
	c := &automaticCatalog{}
	c.allowed.Store(true)
	for i := 0; i < n; i++ {
		c.channels = append(c.channels, edgecatalog.Channel{ID: fmt.Sprintf("auto-%02d", i), Protocols: []edgecatalog.Protocol{{ID: "http_ts"}}})
	}
	dir := filepath.Join(t.TempDir(), "live")
	m, e := New(context.Background(), dir, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(m.Close)
	return m, c, dir
}
func TestAutomaticReceptionQueuesAndHonorsPersistentStop(t *testing.T) {
	m, c, dir := automaticManager(t, 18)
	c.channels = append(c.channels, edgecatalog.Channel{ID: "local_synthetic", Protocols: []edgecatalog.Protocol{{ID: "http_ts"}}})
	m.reconcileAutomatic()
	waitStatus(t, m, "running")
	if len(m.jobs) != 16 || len(m.Snapshot().Configuration.Channels) != 18 {
		t.Fatal("admission/manual isolation", len(m.jobs), m.Snapshot().Configuration)
	}
	queued := 0
	for _, s := range m.Snapshot().Streams {
		if s.State == "queued" {
			queued++
		}
	}
	if queued != 2 {
		t.Fatal("missing queued channels", queued)
	}
	if e := m.Stop("auto-00"); e != nil {
		t.Fatal(e)
	}
	m.reconcileAutomatic()
	if m.IsActive("auto-00") || !m.IsActive("auto-16") {
		t.Fatal("stop overridden or queue not admitted")
	}
	m.Close()
	next, e := New(context.Background(), dir, c)
	if e != nil {
		t.Fatal(e)
	}
	defer next.Close()
	next.Resume()
	next.reconcileAutomatic()
	if next.IsActive("auto-00") {
		t.Fatal("manual stop lost on restart")
	}
}
func TestActiveConfigurationValidatedBeforeRestartAndOtherChannelsStayOpen(t *testing.T) {
	m, c, _ := automaticManager(t, 2)
	m.reconcileAutomatic()
	waitStatus(t, m, "running")
	old := m.jobs["auto-00"]
	other := m.jobs["auto-01"]
	config := m.Snapshot().Configuration
	entry := config.Channels[0]
	entry.Protocol = "unsupported"
	if e := m.Save(entry, config.Tools, config.Generation); e == nil {
		t.Fatal("invalid edit accepted")
	}
	if m.jobs["auto-00"] != old || !m.IsActive("auto-00") {
		t.Fatal("invalid edit interrupted stream")
	}
	entry = config.Channels[0]
	entry.Resume = false
	if e := m.Save(entry, config.Tools, config.Generation); e != nil {
		t.Fatal(e)
	}
	if m.jobs["auto-00"] == old || m.jobs["auto-01"] != other || !m.IsActive("auto-01") {
		t.Fatal("restart isolation")
	}
	before := c.opens.Load()
	m.reconcileAutomatic()
	if c.opens.Load() > before+1 {
		t.Fatal("automatic pass duplicated sources")
	}
}
func TestDirectStartPreparesInputAndRevocationIsEnforced(t *testing.T) {
	m, c, _ := automaticManager(t, 1)
	if e := m.StartReviewed("auto-00", 0); e != nil {
		t.Fatal(e)
	}
	waitStatus(t, m, "running")
	config := m.Snapshot().Configuration
	if len(config.Channels) != 1 || config.Channels[0].HLS || config.Channels[0].UDP != nil {
		t.Fatal("implicit outputs")
	}
	if e := m.Stop("auto-00"); e != nil {
		t.Fatal(e)
	}
	c.allowed.Store(false)
	if e := m.StartReviewed("auto-00", m.Snapshot().Configuration.Generation); e != ErrDenied {
		t.Fatal("revoked source started", e)
	}
}

func TestAutomaticDoesNotActivatePreviouslyUnapprovedOutputs(t *testing.T) {
	m, c, _ := automaticManager(t, 1)
	config := m.store.snapshot()
	config.Channels = append(config.Channels, ChannelConfig{ChannelID: "auto-00", Protocol: "http_ts", HLS: true})
	if e := m.store.replace(config, config.Generation); e != nil {
		t.Fatal(e)
	}
	m.reconcileAutomatic()
	if m.IsActive("auto-00") || c.opens.Load() != 0 {
		t.Fatal("prepared output activated implicitly")
	}
}

func TestAutomaticWorkerStartsWithoutBrowserAndCloses(t *testing.T) {
	m, _, _ := automaticManager(t, 1)
	m.RunAutomatic()
	m.RunAutomatic()
	waitStatus(t, m, "running")
	m.Close()
	if m.IsActive("auto-00") {
		t.Fatal("worker/source survived close")
	}
}
