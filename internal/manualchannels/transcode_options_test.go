package manualchannels

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func advancedProfile() Transcode {
	return Transcode{Mode: "h264", Device: "cpu", FPS: 25, Audio: "aac", AudioKbps: 128, SampleRate: 48000, Channels: 2, Tracks: []VideoTrack{{Width: 640, Height: 360, VideoKbps: 800, Profile: "main", Preset: "veryfast", BFrames: 2, Refs: 2, SAR: "1:1", Resize: "fit", Background: "#101820"}, {Width: 320, Height: 180, VideoKbps: 400, Resize: "crop"}}}
}
func TestAdvancedProcessingPersistsAndRejectsUnsafeOptions(t *testing.T) {
	s, id := processingFixture(t)
	v := advancedProfile()
	if e := s.SaveProcessing(id, nil, &v, s.Revision()); e != nil {
		t.Fatal(e)
	}
	saved, e := s.Processing(id)
	if e != nil || len(saved.Transcode.Tracks) != 2 {
		t.Fatal("tracks not persisted")
	}
	cases := []func(*Transcode){func(p *Transcode) { p.Tracks[0].Background = "red,movie=/etc/passwd" }, func(p *Transcode) { p.Crop = "100:100:0:0,movie=http://private" }, func(p *Transcode) { p.Tracks[0].Preset = "-i" }, func(p *Transcode) { p.Tracks[0].SAR = "movie=x" }, func(p *Transcode) { p.Tracks[0].Codec = "av1" }, func(p *Transcode) { p.Device = "vaapi:renderD128" }, func(p *Transcode) { p.VolumeDB = 21 }, func(p *Transcode) { p.Tracks = append(p.Tracks, p.Tracks...) }, func(p *Transcode) { p.Tracks[0].Profile = "baseline" }, func(p *Transcode) { p.Mode = "copy" }}
	for _, change := range cases {
		p := advancedProfile()
		change(&p)
		if validateTranscode(p) {
			t.Fatal("unsafe or unsupported option accepted")
		}
	}
	ins := []Input{{Key: "primary", Protocol: "http_ts", SourceTimeout: 12}}
	if e = s.SaveProcessing(id, &ins, nil, s.Revision()); e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	s.activeInput[id] = "primary"
	s.mu.Unlock()
	if s.SourceIdleTimeout(id) != 12 {
		t.Fatal("source timeout not used")
	}
	ins[0].SourceTimeout = 301
	if e = s.SaveProcessing(id, &ins, nil, s.Revision()); e == nil {
		t.Fatal("unbounded watchdog")
	}
	args := strings.Join(transcodeArgs(v), " ")
	for _, part := range []string{"-filter:v:1", "-bf:v:0 2", "-refs:v:0 2", "-profile:v:0 main", "-ar 48000"} {
		if !strings.Contains(args, part) {
			t.Fatal("option missing", part)
		}
	}
}

func TestNativeAdvancedTranscode(t *testing.T) {
	tool := os.Getenv("KORE_MANUAL_FFMPEG")
	if tool == "" {
		t.Skip("set KORE_MANUAL_FFMPEG for native synthetic validation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	sample, e := exec.CommandContext(ctx, tool, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=640x360:rate=25", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-threads", "1", "-c:a", "aac", "-ac", "2", "-f", "mpegts", "pipe:1").Output()
	if e != nil {
		t.Fatal("synthetic generation", e)
	}
	configs := []Transcode{advancedProfile(), {Mode: "h264", Device: "cpu", FPS: 25, VideoCopy: true, Audio: "aac", AudioKbps: 96, SampleRate: 44100, Channels: 1, VolumeDB: -3}, advancedProfile()}
	configs[0].Tracks[0].Extended = []EncoderOption{{Key: "tune", Value: "film"}, {Key: "scene_threshold", Value: "0"}, {Key: "min_keyframe", Value: "25"}, {Key: "slices", Value: "2"}}
	configs[2].Tracks = configs[2].Tracks[:1]
	configs[2].SplitChannels = true
	configs[2].Deinterlace = "bwdif"
	configs[2].Crop = "640:360:0:0"
	configs[2].GOP = 25
	for i, p := range configs {
		s, _ := processingFixture(t)
		reader, e := s.transcode(ctx, io.NopCloser(bytes.NewReader(sample)), tool, p)
		if e != nil {
			t.Fatal(e)
		}
		out, e := io.ReadAll(io.LimitReader(reader, 8<<20))
		reader.Close()
		if e != nil {
			t.Fatalf("profile %d: %v", i, e)
		}
		probe := exec.CommandContext(ctx, filepath.Join(filepath.Dir(tool), "ffprobe"), "-v", "error", "-show_entries", "stream=codec_type,width,height,channels,sample_rate", "-of", "json", "-i", "pipe:0")
		probe.Stdin = bytes.NewReader(out)
		info, e := probe.Output()
		if e != nil {
			t.Fatalf("profile %d probe: %v", i, e)
		}
		var result struct {
			Streams []struct {
				Kind       string `json:"codec_type"`
				Width      int    `json:"width"`
				Height     int    `json:"height"`
				Channels   int    `json:"channels"`
				SampleRate string `json:"sample_rate"`
			} `json:"streams"`
		}
		if json.Unmarshal(info, &result) != nil {
			t.Fatal("probe decode")
		}
		video, audio := 0, 0
		for _, track := range result.Streams {
			if track.Kind == "video" {
				video++
				if i == 0 && video == 2 && track.Width != 320 {
					t.Fatal("second rendition not applied")
				}
			}
			if track.Kind == "audio" {
				audio++
				if i == 1 && (track.Channels != 1 || track.SampleRate != "44100") {
					t.Fatal("audio-only controls not applied")
				}
				if i == 2 && track.Channels != 1 {
					t.Fatal("split audio not mono")
				}
			}
		}
		expectedVideo, expectedAudio := 1, 1
		if i == 0 {
			expectedVideo = 2
		}
		if i == 2 {
			expectedAudio = 2
		}
		if video != expectedVideo || audio != expectedAudio {
			t.Fatalf("profile %d got %d video/%d audio", i, video, audio)
		}
		if len(s.transcodeSlots) != 0 {
			t.Fatal("encoder resource leaked")
		}
	}
}
