// Package hls remuxes fixed synthetic H.264/AAC sources without transcoding.
package hls

import (
	"errors"
	"path/filepath"
)

var ErrUnavailable = errors.New("HLS unavailable")

type Config struct {
	Enabled bool   `yaml:"enabled"`
	FFmpeg  string `yaml:"ffmpeg"`
	FFprobe string `yaml:"ffprobe"`
	Root    string `yaml:"root"`
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !filepath.IsAbs(c.FFmpeg) || !filepath.IsAbs(c.FFprobe) || !filepath.IsAbs(c.Root) || filepath.Dir(filepath.Clean(c.Root)) == filepath.Clean(c.Root) {
		return errors.New("HLS requires absolute tool paths and dedicated temporary directory")
	}
	return nil
}

// SyntheticArgs creates media, it does not transcode an incoming stream.
func SyntheticArgs(realtime bool) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if realtime {
		args = append(args, "-re")
	}
	args = append(args, "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-map", "0:v:0", "-map", "1:a:0", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-tune", "zerolatency", "-pix_fmt", "yuv420p", "-g", "50", "-keyint_min", "50", "-sc_threshold", "0", "-b:v", "400k", "-maxrate", "500k", "-bufsize", "1000k", "-c:a", "aac", "-b:a", "64k", "-ac", "1")
	if !realtime {
		args = append(args, "-t", "2")
	}
	return append(args, "-f", "mpegts", "pipe:1")
}
func segmentArgs() []string {
	return []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-protocol_whitelist", "pipe,file", "-analyzeduration", "1000000", "-probesize", "1000000", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:0", "-c", "copy", "-f", "hls", "-hls_time", "2", "-hls_list_size", "4", "-hls_start_number_source", "epoch_us", "-hls_delete_threshold", "1", "-hls_flags", "delete_segments+temp_file+omit_endlist+independent_segments+discont_start", "-hls_segment_filename", "seg%09d.ts", "index.m3u8"}
}
