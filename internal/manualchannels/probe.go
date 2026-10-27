package manualchannels

import (
	"bytes"
	"context"
	"io"
	"strings"
	"time"
)

func transport(data []byte) bool {
	if len(data) < 564 {
		return false
	}
	for i := 0; i < 188; i++ {
		if data[i] == 0x47 && data[i+188] == 0x47 && data[i+376] == 0x47 {
			return true
		}
	}
	return false
}

// Probe is explicit, bounded and never saves or starts a continuous receiver.
func (s *Service) Probe(parent context.Context, input Source, ffmpeg string) (Detection, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return Detection{}, ErrConflict
	}
	ctx, cancel := context.WithTimeout(parent, 7*time.Second)
	defer cancel()
	if input.ID != "" && input.URL == "" {
		current, ok := s.source(input.ID)
		if !ok {
			return Detection{}, ErrInvalid
		}
		input = current
	}
	d, e := Detect(input.URL, input.Protocol)
	if e != nil {
		return Detection{}, e
	}
	if d.Protocol == "" {
		response, e := s.openHTTP(ctx, input.URL, "")
		if e != nil {
			return Detection{}, e
		}
		data, readError := io.ReadAll(io.LimitReader(response.Body, 564))
		response.Body.Close()
		if readError != nil {
			return Detection{}, ErrUnavailable
		}
		if bytes.HasPrefix(data, []byte("#EXTM3U")) {
			d.Protocol = "hls"
		} else if transport(data) {
			d.Protocol = "http_ts"
			d.Verified = true
			d.Detail = "Pacotes MPEG-TS recebidos. Codecs e qualidade de imagem ainda não verificados."
			return d, nil
		} else {
			return Detection{}, ErrInvalid
		}
	}
	input.Protocol = d.Protocol
	source, e := normalize(input)
	if e != nil {
		return Detection{}, e
	}
	var reader io.ReadCloser
	switch d.Protocol {
	case "http_ts":
		response, err := s.openHTTP(ctx, source.URL, "")
		e = err
		if response != nil {
			reader = response.Body
		}
	case "hls":
		reader, e = s.openHLS(ctx, source, ffmpeg)
	case "srt":
		reader, e = s.openSRT(ctx, source)
	case "udp":
		reader, e = s.openUDP(ctx, source)
	}
	if e != nil || reader == nil {
		return Detection{}, ErrUnavailable
	}
	reader = contextual(ctx, reader)
	defer reader.Close()
	sample := make([]byte, 564)
	if _, e = io.ReadFull(reader, sample); e != nil || !transport(sample) {
		return Detection{}, ErrUnavailable
	}
	d.Verified = true
	d.Detail = "Pacotes " + strings.ToUpper(d.Protocol) + " recebidos; transporte MPEG-TS válido. Imagem, áudio e estabilidade ainda precisam ser conferidos no player."
	return d, nil
}
