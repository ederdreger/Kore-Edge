package manualchannels

import (
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// Typed controls only: no shell arguments, arbitrary filters or file paths.
type VideoTrack struct {
	VideoKbps  int    `json:"video_kbps"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Codec      string `json:"codec,omitempty"`
	Profile    string `json:"profile,omitempty"`
	Preset     string `json:"preset,omitempty"`
	Level      string `json:"level,omitempty"`
	BFrames    int    `json:"bframes,omitempty"`
	Refs       int    `json:"refs,omitempty"`
	SAR        string `json:"sar,omitempty"`
	Resize     string `json:"resize,omitempty"`
	Background string `json:"background,omitempty"`
	Interlace  bool   `json:"interlace,omitempty"`
	OpenGOP    bool   `json:"open_gop,omitempty"`
}

func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func videoTracks(t Transcode) []VideoTrack {
	if len(t.Tracks) > 0 {
		return t.Tracks
	}
	return []VideoTrack{{VideoKbps: t.VideoKbps, Width: t.Width, Height: t.Height}}
}
func validTranscode(t Transcode) bool {
	if t.Mode == "copy" {
		return reflect.DeepEqual(t, Transcode{Mode: "copy"})
	}
	if t.Mode != "h264" || !(t.Device == "cpu" || regexp.MustCompile(`^(nvidia:[0-9]{1,2}|vaapi:renderD[0-9]{3})$`).MatchString(t.Device)) {
		return false
	}
	if t.FPS < 1 || t.FPS > 60 || t.GOP < 0 || t.GOP > 600 || !oneOf(t.Deinterlace, "", "off", "yadif", "bwdif") || !oneOf(t.PixelFormat, "", "yuv420p") {
		return false
	}
	if t.Crop != "" {
		parts := strings.Split(t.Crop, ":")
		if len(parts) != 4 {
			return false
		}
		for i, p := range parts {
			n, e := strconv.Atoi(p)
			if e != nil || n < 0 || n > 8192 || n%2 != 0 || i < 2 && n < 2 {
				return false
			}
		}
	}
	if t.Audio != "copy" && !(t.Audio == "aac" && t.AudioKbps >= 32 && t.AudioKbps <= 320) {
		return false
	}
	if !oneOf(strconv.Itoa(t.SampleRate), "0", "32000", "44100", "48000") || t.Channels != 0 && t.Channels != 1 && t.Channels != 2 || math.IsNaN(t.VolumeDB) || math.IsInf(t.VolumeDB, 0) || t.VolumeDB < -30 || t.VolumeDB > 20 {
		return false
	}
	if t.Audio == "copy" && (t.SampleRate != 0 || t.Channels != 0 || t.VolumeDB != 0 || t.SplitChannels) {
		return false
	}
	if t.SplitChannels && t.Channels != 2 {
		return false
	}
	if t.VideoCopy {
		return len(t.Tracks) == 0 && t.Deinterlace == "" && t.Crop == "" && t.GOP == 0 && t.PixelFormat == "" && t.Audio != "copy"
	}
	tracks := videoTracks(t)
	if len(tracks) > 3 {
		return false
	}
	for _, v := range tracks {
		if v.VideoKbps < 200 || v.VideoKbps > 30000 || v.Width < 160 || v.Width > 3840 || v.Width%2 != 0 || v.Height < 90 || v.Height > 2160 || v.Height%2 != 0 {
			return false
		}
		if !oneOf(v.Codec, "", "h264") || !oneOf(v.Profile, "", "baseline", "main", "high") || !oneOf(v.Preset, "", "ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow") || !oneOf(v.Level, "", "3.0", "3.1", "4.0", "4.1", "4.2", "5.0", "5.1") || v.BFrames < 0 || v.BFrames > 4 || v.Refs < 0 || v.Refs > 6 || !oneOf(v.SAR, "", "1:1", "4:3", "16:9", "64:45", "16:11") || !oneOf(v.Resize, "", "fit", "stretch", "crop") || v.Background != "" && !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(v.Background) {
			return false
		}
		if v.Profile == "baseline" && (v.BFrames != 0 || v.Interlace) {
			return false
		}
		// CPU-only controls are rejected rather than silently ignored by GPU drivers.
		if t.Device != "cpu" && (v.Interlace || v.OpenGOP || v.Refs != 0 || v.Level != "" || v.Profile == "baseline") {
			return false
		}
		if strings.HasPrefix(t.Device, "vaapi:") && (v.Preset != "" || v.Profile != "" || v.BFrames != 0) {
			return false
		}
	}
	return true
}

func trackFilter(t Transcode, v VideoTrack) string {
	filters := []string{}
	if t.Deinterlace != "" && t.Deinterlace != "off" {
		filters = append(filters, t.Deinterlace+"=mode=send_frame:parity=auto:deint=interlaced")
	}
	if t.Crop != "" {
		filters = append(filters, "crop="+t.Crop)
	}
	size := strconv.Itoa(v.Width) + ":" + strconv.Itoa(v.Height)
	switch v.Resize {
	case "stretch":
		filters = append(filters, "scale="+size)
	case "crop":
		filters = append(filters, "scale="+size+":force_original_aspect_ratio=increase", "crop="+size)
	default:
		color := "black"
		if v.Background != "" {
			color = "0x" + strings.TrimPrefix(v.Background, "#")
		}
		filters = append(filters, "scale="+size+":force_original_aspect_ratio=decrease", "pad="+size+":(ow-iw)/2:(oh-ih)/2:color="+color)
	}
	sar := v.SAR
	if sar == "" {
		sar = "1:1"
	}
	filters = append(filters, "setsar="+strings.ReplaceAll(sar, ":", "/"))
	if v.Interlace {
		filters = append(filters, "interlace=scan=tff:lowpass=complex")
	}
	filters = append(filters, "format=yuv420p")
	if strings.HasPrefix(t.Device, "vaapi:") {
		filters = append(filters, "format=nv12", "hwupload")
	}
	return strings.Join(filters, ",")
}

func configuredTranscodeArgs(t Transcode) []string {
	args := append([]string{"-hide_banner", "-loglevel", "quiet", "-nostdin"}, deviceArgs(t.Device)...)
	args = append(args, "-protocol_whitelist", "pipe", "-f", "mpegts", "-analyzeduration", "2000000", "-probesize", "2000000", "-i", "pipe:0")
	if t.VideoCopy {
		args = append(args, "-map", "0:v:0", "-c:v", "copy")
	} else {
		for i, v := range videoTracks(t) {
			suffix := ":v:" + strconv.Itoa(i)
			args = append(args, "-map", "0:v:0", "-filter"+suffix, trackFilter(t, v))
			encoder, preset := "libx264", "veryfast"
			if strings.HasPrefix(t.Device, "nvidia:") {
				encoder, preset = "h264_nvenc", "p4"
				args = append(args, "-gpu"+suffix, strings.TrimPrefix(t.Device, "nvidia:"))
			}
			if strings.HasPrefix(t.Device, "vaapi:") {
				encoder, preset = "h264_vaapi", ""
			}
			if v.Preset != "" {
				preset = v.Preset
				if encoder == "h264_nvenc" {
					preset = map[string]string{"ultrafast": "p1", "superfast": "p2", "veryfast": "p3", "faster": "p4", "fast": "p4", "medium": "p5", "slow": "p6"}[v.Preset]
				}
			}
			args = append(args, "-c"+suffix, encoder)
			if preset != "" {
				args = append(args, "-preset"+suffix, preset)
			}
			if t.Device == "cpu" {
				args = append(args, "-threads"+suffix, "1")
			}
			gop := t.GOP
			if gop == 0 {
				gop = t.FPS * 2
			}
			args = append(args, "-b"+suffix, strconv.Itoa(v.VideoKbps)+"k", "-maxrate"+suffix, strconv.Itoa(v.VideoKbps)+"k", "-bufsize"+suffix, strconv.Itoa(v.VideoKbps*2)+"k", "-r"+suffix, strconv.Itoa(t.FPS), "-g"+suffix, strconv.Itoa(gop), "-bf"+suffix, strconv.Itoa(v.BFrames))
			if v.Profile != "" {
				args = append(args, "-profile"+suffix, v.Profile)
			}
			if v.Level != "" {
				args = append(args, "-level"+suffix, v.Level)
			}
			if v.Refs > 0 {
				args = append(args, "-refs"+suffix, strconv.Itoa(v.Refs))
			}
			if v.Interlace {
				args = append(args, "-flags"+suffix, "+ildct+ilme", "-top"+suffix, "1")
			}
			if t.Device == "cpu" {
				value := "open-gop=0"
				if v.OpenGOP {
					value = "open-gop=1"
				}
				args = append(args, "-x264-params"+suffix, value)
			}
		}
	}
	if t.Audio == "copy" {
		args = append(args, "-map", "0:a?", "-c:a", "copy")
	} else {
		rate, channels := t.SampleRate, t.Channels
		if rate == 0 {
			rate = 48000
		}
		if channels == 0 {
			channels = 2
		}
		if t.SplitChannels {
			for i := 0; i < 2; i++ {
				suffix := ":a:" + strconv.Itoa(i)
				filter := "pan=mono|c0=c" + strconv.Itoa(i)
				if t.VolumeDB != 0 {
					filter += ",volume=" + strconv.FormatFloat(t.VolumeDB, 'f', 2, 64) + "dB"
				}
				args = append(args, "-map", "0:a:0", "-filter"+suffix, filter, "-c"+suffix, "aac", "-b"+suffix, strconv.Itoa(t.AudioKbps)+"k", "-ar"+suffix, strconv.Itoa(rate), "-ac"+suffix, "1")
			}
		} else {
			args = append(args, "-map", "0:a?", "-c:a", "aac", "-b:a", strconv.Itoa(t.AudioKbps)+"k", "-ar", strconv.Itoa(rate), "-ac", strconv.Itoa(channels))
			if t.VolumeDB != 0 {
				args = append(args, "-af", "volume="+strconv.FormatFloat(t.VolumeDB, 'f', 2, 64)+"dB")
			}
		}
	}
	return append(args, "-mpegts_flags", "+resend_headers+initial_discontinuity", "-flush_packets", "1", "-f", "mpegts", "pipe:1")
}
