package hls

import (
	"bytes"
	"context"
	"encoding/json"
	"time"
)

// NewAuthorized creates storage for an authorized real input. The caller must
// validate codecs from that input with ValidateInput before opening a session.
// The laboratory New/Verify contract remains unchanged.
func NewAuthorized(c Config, allow Authorizer) (*Manager, error) {
	if c.Validate() != nil || !c.Enabled || allow == nil {
		return nil, ErrUnavailable
	}
	return newStorage(c, allow)
}
func ValidateInput(ctx context.Context, c Config, sample []byte) error {
	if c.Validate() != nil || len(sample) == 0 || len(sample) > 4*1024*1024 {
		return ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	data, e := commandOutput(ctx, c.FFprobe, []string{"-v", "error", "-protocol_whitelist", "pipe", "-f", "mpegts", "-analyzeduration", "1500000", "-probesize", "2000000", "-show_entries", "stream=codec_type,codec_name", "-of", "json", "pipe:0"}, bytes.NewReader(sample), 64*1024)
	if e != nil {
		return ErrUnavailable
	}
	return validateSelectedCodecs(data)
}

// The output maps the first video and audio tracks; additional tracks are not
// transcoded and do not change the codecs of those selected tracks.
func validateSelectedCodecs(data []byte) error {
	var info struct {
		Streams []struct {
			Type  string `json:"codec_type"`
			Codec string `json:"codec_name"`
		} `json:"streams"`
	}
	if json.Unmarshal(data, &info) != nil {
		return ErrUnavailable
	}
	video, audio := "", ""
	for _, stream := range info.Streams {
		if stream.Type == "video" && video == "" {
			video = stream.Codec
		}
		if stream.Type == "audio" && audio == "" {
			audio = stream.Codec
		}
	}
	if video != "h264" || audio != "aac" {
		return ErrUnavailable
	}
	return nil
}
