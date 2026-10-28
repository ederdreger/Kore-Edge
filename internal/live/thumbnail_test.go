package live

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"kore-edge/internal/mediaresources"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func thumbnailFixture(t *testing.T) (*Manager, *syntheticCatalog) {
	t.Helper()
	catalog := &syntheticCatalog{}
	catalog.allowed.Store(true)
	m, err := New(context.Background(), filepath.Join(t.TempDir(), "live"), catalog)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	if m.Save(ChannelConfig{ChannelID: "synthetic", Protocol: "http_ts"}, Tools{}, 0) != nil || m.Start("synthetic") != nil {
		t.Fatal("setup")
	}
	waitStatus(t, m, "running")
	return m, catalog
}

func TestThumbnailSharesOutputCachesAndRejectsRevocation(t *testing.T) {
	m, catalog := thumbnailFixture(t)
	var calls atomic.Int32
	m.thumbnails.generate = func(ctx context.Context, id string) ([]byte, error) {
		calls.Add(1)
		input, err := m.SharedStream(ctx, id)
		if err != nil {
			return nil, err
		}
		defer input.Close()
		if _, err = io.ReadFull(input, make([]byte, 1316)); err != nil {
			return nil, err
		}
		var data bytes.Buffer
		err = jpeg.Encode(&data, image.NewRGBA(image.Rect(0, 0, 480, 270)), nil)
		return data.Bytes(), err
	}
	first, err := m.Thumbnail(context.Background(), "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Thumbnail(context.Background(), "synthetic")
	if err != nil || !bytes.Equal(first, second) || calls.Load() != 1 || catalog.opens.Load() != 1 {
		t.Fatal("thumbnail reopened source or ignored cache")
	}
	m.thumbnails.mu.Lock()
	m.thumbnails.entries["synthetic"].at = time.Now().Add(-thumbnailTTL)
	m.thumbnails.mu.Unlock()
	if _, err = m.Thumbnail(context.Background(), "synthetic"); err != nil || calls.Load() != 2 || catalog.opens.Load() != 1 {
		t.Fatal("expired image not refreshed from shared output")
	}
	catalog.allowed.Store(false)
	if _, err = m.Thumbnail(context.Background(), "synthetic"); !errors.Is(err, ErrDenied) {
		t.Fatal("cached image served after revocation")
	}
}

func TestThumbnailCoalescesConcurrentRequests(t *testing.T) {
	m, _ := thumbnailFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	m.thumbnails.generate = func(ctx context.Context, _ string) ([]byte, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
			return []byte("synthetic"), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := m.Thumbnail(context.Background(), "synthetic"); err != nil {
			t.Error(err)
		}
	}()
	<-entered
	go func() {
		defer wg.Done()
		if _, err := m.Thumbnail(context.Background(), "synthetic"); err != nil {
			t.Error(err)
		}
	}()
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("duplicate image decoding")
	}
}

func TestThumbnailBusyAndStoppedNeverOpenAnOrigin(t *testing.T) {
	m, catalog := thumbnailFixture(t)
	thumbnailSlots <- struct{}{}
	thumbnailSlots <- struct{}{}
	_, err := m.Thumbnail(context.Background(), "synthetic")
	<-thumbnailSlots
	<-thumbnailSlots
	if !errors.Is(err, mediaresources.ErrBusy) || catalog.opens.Load() != 1 {
		t.Fatal("unbounded thumbnail admission")
	}
	if m.Stop("synthetic") != nil {
		t.Fatal("stop")
	}
	if _, err = m.Thumbnail(context.Background(), "synthetic"); !errors.Is(err, ErrDenied) || catalog.opens.Load() != 1 {
		t.Fatal("thumbnail started stopped source")
	}
}
