package manualchannels

import (
	"context"
	"io"
	"kore-edge/internal/mediaresources"
	"net"
	"sort"
	"strings"
	"time"
)

func concatToken(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// Keep the endpoint and AVOptions on stdin, never argv or a temporary file.
// FFmpeg options preserve #!:: stream IDs and passphrases as decoded values.
func srtManifest(endpoint string, options map[string]string) (string, error) {
	if strings.ContainsAny(endpoint, "\r\n\x00") {
		return "", ErrInvalid
	}
	keys := make([]string, 0, len(options))
	for key, value := range options {
		if !strings.Contains("|mode|transtype|srt_streamid|payload_size|passphrase|pbkeylen|latency|rcvlatency|peerlatency|connect_timeout|timeout|", "|"+key+"|") {
			return "", ErrInvalid
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return "", ErrInvalid
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var text strings.Builder
	text.WriteString("ffconcat version 1.0\nfile " + concatToken(endpoint) + "\n")
	for _, key := range keys {
		text.WriteString("option " + key + " " + concatToken(options[key]) + "\n")
	}
	return text.String(), nil
}

func (s *Service) openNativeSRT(ctx context.Context, source Source, ffmpeg string) (io.ReadCloser, error) {
	u, err := parse(source.URL)
	if err != nil || u.Scheme != "srt" {
		return nil, ErrInvalid
	}
	ip, err := s.resolve(ctx, u.Hostname())
	if err != nil {
		return nil, err
	}
	options := map[string]string{"mode": "caller", "transtype": "live", "latency": "200000", "connect_timeout": "3000", "timeout": "8000000"}
	for key, values := range u.Query() {
		if key == "streamid" {
			key = "srt_streamid"
		}
		if key == "pkt_size" {
			key = "payload_size"
		}
		options[key] = values[0]
	}
	if options["passphrase"] != "" && options["pbkeylen"] == "" {
		options["pbkeylen"] = "32"
	}
	manifest, err := srtManifest("srt://"+net.JoinHostPort(ip, u.Port()), options)
	if err != nil {
		return nil, err
	}
	return s.startNativeSRT(ctx, ffmpeg, manifest)
}

type nativeSRTInput struct{ *processInput }

func (*nativeSRTInput) ReceiverName() string { return "FFmpeg/libsrt" }

func (s *Service) startNativeSRT(parent context.Context, ffmpeg, manifest string) (io.ReadCloser, error) {
	if !executable(ffmpeg) {
		return nil, ErrUnavailable
	}
	release, ok := mediaresources.Input()
	if !ok {
		return nil, mediaresources.ErrBusy
	}
	ctx, cancel := context.WithCancel(parent)
	cmd := s.command(ctx, ffmpeg, "-hide_banner", "-loglevel", "quiet", "-nostdin", "-protocol_whitelist", "pipe,srt,udp", "-format_whitelist", "concat,mpegts", "-f", "concat", "-safe", "0", "-i", "pipe:0", "-map", "0", "-c", "copy", "-mpegts_flags", "+resend_headers", "-flush_packets", "1", "-f", "mpegts", "pipe:1")
	mediaresources.Configure(cmd)
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	output, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		release()
		return nil, ErrUnavailable
	}
	if cmd.Start() != nil {
		cancel()
		output.Close()
		release()
		return nil, ErrUnavailable
	}
	p := &processInput{output: output, cancel: cancel, done: make(chan struct{})}
	go func() { cmd.Wait(); release(); close(p.done); cancel() }()
	return &nativeSRTInput{p}, nil
}
