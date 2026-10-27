package live

import (
	"encoding/json"
	"kore-edge/internal/catalog"
	"kore-edge/internal/multicast"
	"kore-edge/internal/security"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
)

var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type Tools struct {
	FFmpeg  string `json:"ffmpeg"`
	FFprobe string `json:"ffprobe"`
}
type ChannelConfig struct {
	ChannelID  string            `json:"channel_id"`
	Protocol   string            `json:"protocol"`
	UDP        *multicast.Target `json:"udp,omitempty"`
	HLS        bool              `json:"hls"`
	AutoPaused bool              `json:"auto_paused,omitempty"`
	Resume     bool              `json:"resume"`
}
type Configuration struct {
	Version    int             `json:"version"`
	Generation uint64          `json:"generation"`
	Tools      Tools           `json:"tools"`
	Channels   []ChannelConfig `json:"channels"`
}
type configStore struct {
	mu        sync.Mutex
	directory string
	data      Configuration
}

func loadConfig(directory string) (*configStore, error) {
	if security.PrivateDirectory(directory, true) != nil {
		return nil, ErrUnavailable
	}
	store := &configStore{directory: directory, data: Configuration{Version: 1, Channels: []ChannelConfig{}}}
	file := filepath.Join(directory, "distribution.json")
	if _, e := os.Lstat(file); !os.IsNotExist(e) {
		data, e := security.ReadPrivateFile(file, 512*1024)
		if e != nil || catalog.DecodeStrict(data, &store.data) != nil || store.data.validate() != nil {
			return nil, ErrUnavailable
		}
	}
	return store, nil
}
func trustedTool(path, name string) bool {
	if path == "" {
		return true
	}
	if !filepath.IsAbs(path) {
		return false
	}
	base := filepath.Base(path)
	if base != name && base != name+".exe" {
		return false
	}
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return false
	}
	info, e := os.Stat(resolved)
	return e == nil && info.Mode().IsRegular() && (os.PathSeparator == '\\' || info.Mode().Perm()&0022 == 0)
}
func (c Configuration) validate() error {
	if c.Version != 1 || len(c.Channels) > 512 || !trustedTool(c.Tools.FFmpeg, "ffmpeg") || !trustedTool(c.Tools.FFprobe, "ffprobe") {
		return ErrInvalid
	}
	seen := map[string]bool{}
	dest := map[string]bool{}
	for _, entry := range c.Channels {
		if !safeID.MatchString(entry.ChannelID) || seen[entry.ChannelID] || (entry.Protocol != "http_ts" && entry.Protocol != "hls" && entry.Protocol != "srt" && entry.Protocol != "udp") {
			return ErrInvalid
		}
		seen[entry.ChannelID] = true
		if entry.UDP != nil {
			if entry.UDP.ChannelID != entry.ChannelID || entry.UDP.Validate() != nil {
				return ErrInvalid
			}
			key := entry.UDP.Group + ":" + strconv.Itoa(entry.UDP.Port)
			if dest[key] {
				return ErrInvalid
			}
			dest[key] = true
		}
	}
	return nil
}
func (s *configStore) snapshot() Configuration {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(s.data)
	var copy Configuration
	_ = json.Unmarshal(data, &copy)
	return copy
}
func (s *configStore) replace(next Configuration, expected uint64) error {
	if next.validate() != nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != s.data.Generation {
		return ErrConflict
	}
	next.Generation = s.data.Generation + 1
	data, e := json.Marshal(next)
	if e != nil {
		return ErrUnavailable
	}
	file, e := os.CreateTemp(s.directory, ".distribution-")
	if e != nil {
		return ErrUnavailable
	}
	defer os.Remove(file.Name())
	if _, e = file.Write(data); e != nil {
		file.Close()
		return ErrUnavailable
	}
	if file.Sync() != nil {
		file.Close()
		return ErrUnavailable
	}
	if file.Close() != nil || os.Rename(file.Name(), filepath.Join(s.directory, "distribution.json")) != nil {
		return ErrUnavailable
	}
	if os.PathSeparator != '\\' {
		dir, e := os.Open(s.directory)
		if e != nil {
			return ErrUnavailable
		}
		e = dir.Sync()
		dir.Close()
		if e != nil {
			return ErrUnavailable
		}
	}
	s.data = next
	return nil
}
