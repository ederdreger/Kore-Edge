package manualchannels

import (
	"bytes"
	"context"
	"io"
	"kore-edge/internal/hls"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestMediaInfoUsesMeasuredFieldsOnly(t *testing.T) {
	info := decodeMedia([]byte(`{"streams":[{"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"tags":{"title":"SECRET"}},{"codec_type":"audio","codec_name":"aac"}]}`))
	if info == nil || info.Video != "h264" || info.Audio != "aac" || info.Width != 1920 || info.Height != 1080 || info.MeasuredAt.IsZero() {
		t.Fatal("missing measured media fields")
	}
	for _, data := range []string{`invalid`, `{"streams":[]}`, `{"streams":[{"codec_type":"video","codec_name":"SECRET URL","width":1920,"height":1080}]}`, `{"streams":[{"codec_type":"video","codec_name":"h264","width":999999,"height":1080}]}`} {
		if decodeMedia([]byte(data)) != nil {
			t.Fatal("accepted invalid media")
		}
	}
}

func TestSampleContinuityObservesDamagedInput(t *testing.T) {
	first, second, third := packet(), packet(), packet()
	first[3], second[3], third[3] = 0x10, 0x12, 0x13
	third[1] |= 0x80
	data := append(append(first, second...), third...)
	info := &MediaInfo{}
	sampleTransport(info, data)
	if info.ContinuityErrors != 1 || info.TransportErrors != 1 {
		t.Fatalf("missing transport damage: %+v", info)
	}
}

func TestMediaSamplingPreservesBytesAndReleasesEarlyClose(t *testing.T) {
	before := len(mediaProbeSlots)
	data := bytes.Repeat([]byte{0x47, 1, 2, 3}, 200)
	reader := observeMedia(context.Background(), io.NopCloser(bytes.NewReader(data)), "unused", func(*MediaInfo) { t.Error("early sample probed") })
	got := make([]byte, len(data))
	if _, err := io.ReadFull(reader, got); err != nil || !bytes.Equal(got, data) {
		t.Fatal("observer changed media")
	}
	if len(mediaProbeSlots) != before+1 {
		t.Fatal("sample not bounded")
	}
	reader.Close()
	reader.Close()
	if len(mediaProbeSlots) != before {
		t.Fatal("sampling slot leaked")
	}
}

func TestMediaSamplingSkipsWhenBusyWithoutBlockingReception(t *testing.T) {
	mediaProbeSlots <- struct{}{}
	mediaProbeSlots <- struct{}{}
	defer func() { <-mediaProbeSlots; <-mediaProbeSlots }()
	data := []byte("synthetic unchanged stream")
	reader := observeMedia(context.Background(), io.NopCloser(bytes.NewReader(data)), "unused", func(*MediaInfo) { t.Error("busy probe started") })
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("resource contention affected reception")
	}
}

func TestNativeMediaObserver(t *testing.T) {
	tool := os.Getenv("KORE_MANUAL_FFMPEG")
	if tool == "" {
		t.Skip("set KORE_MANUAL_FFMPEG for synthetic native media measurement")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sample, err := exec.CommandContext(ctx, tool, hls.SyntheticArgs(false)...).Output()
	if err != nil {
		t.Fatal("synthetic sample", err)
	}
	probe := filepath.Join(filepath.Dir(tool), "ffprobe")
	if runtime.GOOS == "windows" {
		probe += ".exe"
	}
	measured := make(chan *MediaInfo, 1)
	reader := observeMedia(ctx, io.NopCloser(bytes.NewReader(sample)), probe, func(info *MediaInfo) { measured <- info })
	got, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || !bytes.Equal(got, sample) {
		t.Fatal("probe changed stream")
	}
	select {
	case info := <-measured:
		if info.Video != "h264" || info.Audio != "aac" || info.Width < 1 || info.Height < 1 {
			t.Fatal("incorrect measured codecs")
		}
	case <-ctx.Done():
		t.Fatal("no media measurement")
	}
}
