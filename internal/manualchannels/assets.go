package manualchannels

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"kore-edge/internal/mediaresources"
	"kore-edge/internal/security"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"
)

var assetID = regexp.MustCompile(`^[a-f0-9]{64}\.(png|mp4)$`)

type Asset struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Bytes int64  `json:"bytes"`
}

func (s *Service) assetDir() (string, error) {
	p := filepath.Join(s.directory, "assets")
	if security.PrivateDirectory(p, true) != nil {
		return "", ErrUnavailable
	}
	return p, nil
}
func (s *Service) Assets() ([]Asset, error) {
	s.assetsMu.Lock()
	defer s.assetsMu.Unlock()
	return s.assetsLocked()
}
func (s *Service) assetsLocked() ([]Asset, error) {
	dir, e := s.assetDir()
	if e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return nil, ErrUnavailable
	}
	out := []Asset{}
	for _, entry := range entries {
		if assetID.MatchString(entry.Name()) {
			info, e := entry.Info()
			if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				return nil, ErrUnavailable
			}
			kind := "logo"
			if filepath.Ext(entry.Name()) == ".mp4" {
				kind = "backup"
			}
			out = append(out, Asset{entry.Name(), kind, info.Size()})
		}
	}
	return out, nil
}
func (s *Service) assetPath(id, kind string) (string, error) {
	if !assetID.MatchString(id) || kind == "logo" && filepath.Ext(id) != ".png" || kind == "backup" && filepath.Ext(id) != ".mp4" {
		return "", ErrInvalid
	}
	dir, e := s.assetDir()
	if e != nil {
		return "", e
	}
	p := filepath.Join(dir, id)
	f, e := os.Lstat(p)
	if e != nil || !f.Mode().IsRegular() || f.Mode()&os.ModeSymlink != 0 || f.Size() > 64<<20 || runtime.GOOS != "windows" && f.Mode().Perm()&0077 != 0 {
		return "", ErrUnavailable
	}
	return p, nil
}
func (s *Service) ImportAsset(parent context.Context, kind, raw, ffprobe string) (Asset, error) {
	var empty Asset
	if kind != "logo" && kind != "backup" {
		return empty, ErrInvalid
	}
	s.assetsMu.Lock()
	if s.imports == nil {
		s.imports = make(chan struct{}, 1)
	}
	slots := s.imports
	s.assetsMu.Unlock()
	select {
	case slots <- struct{}{}:
		defer func() { <-slots }()
	default:
		return empty, ErrConflict
	}
	ctx, cancel := context.WithTimeout(parent, 50*time.Second)
	defer cancel()
	response, e := s.openHTTP(ctx, raw, "")
	if e != nil {
		return empty, ErrUnavailable
	}
	defer response.Body.Close()
	dir, e := s.assetDir()
	if e != nil {
		return empty, e
	}
	f, e := os.CreateTemp(dir, ".import-")
	if e != nil {
		return empty, ErrUnavailable
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if f.Chmod(0600) != nil {
		return empty, ErrUnavailable
	}
	limit := int64(64 << 20)
	if kind == "logo" {
		limit = 2 << 20
	}
	n, e := io.Copy(f, io.LimitReader(response.Body, limit+1))
	if e != nil || n > limit || n == 0 || ctx.Err() != nil {
		return empty, ErrInvalid
	}
	extension := ".mp4"
	if kind == "logo" {
		extension = ".png"
		if _, e = f.Seek(0, 0); e != nil {
			return empty, ErrUnavailable
		}
		config, format, e := image.DecodeConfig(f)
		if e != nil || format != "png" && format != "jpeg" || config.Width < 1 || config.Height < 1 || config.Width > 2048 || config.Height > 2048 {
			return empty, ErrInvalid
		}
		f.Seek(0, 0)
		decoded, _, e := image.Decode(f)
		if e != nil {
			return empty, ErrInvalid
		}
		var data bytes.Buffer
		if png.Encode(&data, decoded) != nil || data.Len() > 4<<20 {
			return empty, ErrInvalid
		}
		if f.Truncate(0) != nil {
			return empty, ErrUnavailable
		}
		f.Seek(0, 0)
		if _, e = f.Write(data.Bytes()); e != nil {
			return empty, ErrUnavailable
		}
		n = int64(data.Len())
	} else {
		if !executable(ffprobe) {
			return empty, ErrUnavailable
		}
		probeCtx, stop := context.WithTimeout(ctx, 6*time.Second)
		defer stop()
		cmd := s.command(probeCtx, ffprobe, "-v", "quiet", "-protocol_whitelist", "file,pipe", "-f", "mov", "-show_entries", "stream=codec_type,codec_name:format=duration", "-of", "json", f.Name())
		mediaresources.Configure(cmd)
		out := &probeBuffer{}
		cmd.Stdout = out
		cmd.Stderr = io.Discard
		cmd.WaitDelay = time.Second
		if cmd.Run() != nil {
			return empty, ErrInvalid
		}
		var result struct {
			Streams []struct {
				Kind  string `json:"codec_type"`
				Codec string `json:"codec_name"`
			} `json:"streams"`
			Format struct {
				Duration string `json:"duration"`
			} `json:"format"`
		}
		if json.Unmarshal(out.Bytes(), &result) != nil {
			return empty, ErrInvalid
		}
		duration, e := strconv.ParseFloat(result.Format.Duration, 64)
		if e != nil || duration < 1 || duration > 300 || len(result.Streams) > 8 {
			return empty, ErrInvalid
		}
		video := false
		for _, v := range result.Streams {
			if v.Kind == "video" {
				video = true
				if v.Codec != "h264" {
					return empty, ErrInvalid
				}
			}
			if v.Kind == "audio" && v.Codec != "aac" {
				return empty, ErrInvalid
			}
		}
		if !video {
			return empty, ErrInvalid
		}
	}
	if f.Sync() != nil {
		return empty, ErrUnavailable
	}
	f.Seek(0, 0)
	hash := sha256.New()
	if _, e = io.Copy(hash, f); e != nil {
		return empty, ErrUnavailable
	}
	id := hex.EncodeToString(hash.Sum(nil)) + extension
	s.assetsMu.Lock()
	defer s.assetsMu.Unlock()
	assets, e := s.assetsLocked()
	if e != nil {
		return empty, e
	}
	total := n
	for _, a := range assets {
		if a.ID == id {
			return a, nil
		}
		total += a.Bytes
	}
	if len(assets) >= 20 || total > 512<<20 {
		return empty, ErrConflict
	}
	if f.Close() != nil || os.Rename(f.Name(), filepath.Join(dir, id)) != nil {
		return empty, ErrUnavailable
	}
	return Asset{id, kind, n}, nil
}
func (s *Service) DeleteAsset(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.processing.Entries {
		if p.Policy.BackupAsset == id {
			return ErrConflict
		}
		for _, v := range p.Transcode.Tracks {
			if v.Logo == id {
				return ErrConflict
			}
		}
	}
	s.assetsMu.Lock()
	defer s.assetsMu.Unlock()
	p, e := s.assetPath(id, "")
	if e != nil {
		return e
	}
	if os.Remove(p) != nil {
		return ErrUnavailable
	}
	return nil
}
