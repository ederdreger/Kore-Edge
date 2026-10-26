package live

import (
	"bytes"
	"context"
	"io"
	"kore-edge/internal/hls"
	"path/filepath"
	"sync/atomic"
	"time"
)

func (m *Manager) attempt(j *job) error {
	ctx, cancel := context.WithCancel(j.ctx)
	var denied atomic.Bool
	monitorDone := make(chan struct{})
	attemptStart := time.Now().UTC()
	go func() {
		defer close(monitorDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !m.catalog.Authorized(j.config.ChannelID, j.config.Protocol) {
					denied.Store(true)
					cancel()
					return
				}
				j.mu.Lock()
				hub := j.hub
				udp := j.udp
				j.mu.Unlock()
				last := hub.Snapshot().LastPacket
				timeout := 8 * time.Second
				if last.IsZero() {
					last = attemptStart
					timeout = 20 * time.Second
				}
				if time.Since(last) > timeout {
					cancel()
					return
				}
				if udp != nil && !m.catalog.OutputAuthorized(j.config.ChannelID, "udp") {
					_ = udp.Close()
					j.mu.Lock()
					if j.udp == udp {
						j.udp = nil
						j.status.UDP = "unauthorized"
					}
					j.mu.Unlock()
				}
			}
		}
	}()
	defer func() { cancel(); <-monitorDone; j.hlsWG.Wait(); j.closeOutputs() }()
	input, e := m.open(ctx, j.config, j.tools)
	if e != nil {
		return e
	}
	defer input.Close()
	closeDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			input.Close()
		case <-closeDone:
		}
	}()
	defer close(closeDone)
	if j.config.UDP != nil {
		state := "unauthorized"
		if m.catalog.OutputAuthorized(j.config.ChannelID, "udp") {
			interfaces, e := m.interfaces()
			if e == nil {
				sender, e := m.udpOpen(ctx, *j.config.UDP, interfaces)
				if e == nil {
					j.mu.Lock()
					j.udp = sender
					j.mu.Unlock()
					state = "ready"
				} else {
					state = "interface_or_permission_unavailable"
				}
			} else {
				state = "interface_unavailable"
			}
		}
		j.mu.Lock()
		j.status.UDP = state
		j.mu.Unlock()
	}
	initial := make([]byte, 188*3)
	if _, e = io.ReadFull(input, initial); e != nil {
		return e
	}
	offset := -1
	for i := 0; i < 188; i++ {
		if initial[i] == 0x47 && initial[i+188] == 0x47 && initial[i+376] == 0x47 {
			offset = i
			break
		}
	}
	if offset < 0 {
		return ErrInvalid
	}
	reader := io.MultiReader(bytes.NewReader(initial[offset:]), input)
	sample := make([]byte, 0, 2*1024*1024)
	hlsPending := j.config.HLS
	packet := make([]byte, 188)
	for {
		if _, e = io.ReadFull(reader, packet); e != nil {
			if denied.Load() {
				return ErrDenied
			}
			return e
		}
		if packet[0] != 0x47 || packet[3]&0x30 == 0 {
			return ErrInvalid
		}
		j.mu.Lock()
		hub := j.hub
		udp, segmenter := j.udp, j.hlsOutput
		if j.status.FirstPacketAt.IsZero() {
			now := time.Now().UTC()
			elapsed := now.Sub(attemptStart).Milliseconds()
			j.status.FirstPacketAt = now
			j.status.StartupMS = &elapsed
		}
		j.status.State = "running"
		j.status.Error = ""
		j.mu.Unlock()
		if hub.WritePacket(packet) != nil {
			return ErrUnavailable
		}
		if udp != nil {
			if e = udp.WritePacket(packet); e != nil {
				_ = udp.Close()
				j.mu.Lock()
				if j.udp == udp {
					j.udp = nil
					j.status.UDP = "failed"
				}
				j.mu.Unlock()
			} else {
				j.mu.Lock()
				j.status.UDP = "sending"
				j.mu.Unlock()
			}
		}
		if segmenter != nil {
			if e = segmenter.WritePacket(packet); e != nil {
				_ = segmenter.Close()
				j.mu.Lock()
				if j.hlsOutput == segmenter {
					j.hlsOutput = nil
					j.status.HLS = "failed_or_revoked"
				}
				j.mu.Unlock()
			}
		}
		if hlsPending {
			if len(sample) < 2*1024*1024 {
				sample = append(sample, packet...)
			}
			if len(sample) >= 1024*1024 || time.Since(attemptStart) > 3*time.Second {
				hlsPending = false
				m.prepareHLS(ctx, j, sample)
				sample = nil
			}
		}
	}
}
func (j *job) closeOutputs() {
	j.mu.Lock()
	udp, segmenter, manager := j.udp, j.hlsOutput, j.hlsManager
	j.udp = nil
	j.hlsOutput = nil
	j.hlsManager = nil
	j.hlsStarting = false
	if j.status.UDP == "sending" || j.status.UDP == "ready" {
		j.status.UDP = "stopped"
	}
	if j.status.HLS == "running" || j.status.HLS == "validating_codecs" {
		j.status.HLS = "stopped"
	}
	j.mu.Unlock()
	if udp != nil {
		_ = udp.Close()
	}
	if segmenter != nil {
		_ = segmenter.Close()
	}
	if manager != nil {
		manager.Close()
	}
}
func (m *Manager) prepareHLS(ctx context.Context, j *job, sample []byte) {
	j.mu.Lock()
	j.status.HLS = "validating_codecs"
	j.hlsStarting = true
	j.mu.Unlock()
	j.hlsWG.Add(1)
	go func() {
		defer j.hlsWG.Done()
		if !m.catalog.OutputAuthorized(j.config.ChannelID, "hls") {
			j.mu.Lock()
			j.status.HLS = "unauthorized"
			j.mu.Unlock()
			return
		}
		config := hls.Config{Enabled: true, FFmpeg: j.tools.FFmpeg, FFprobe: j.tools.FFprobe, Root: filepath.Join(m.store.directory, "hls")}
		if j.tools.FFmpeg == "" || j.tools.FFprobe == "" || hls.ValidateInput(ctx, config, sample) != nil {
			j.mu.Lock()
			j.status.HLS = "codec_or_tools_unavailable"
			j.mu.Unlock()
			return
		}
		manager, e := hls.NewAuthorized(config, func(id string, _ time.Time) bool { return m.catalog.OutputAuthorized(id, "hls") })
		if e != nil {
			j.mu.Lock()
			j.status.HLS = "storage_unavailable"
			j.mu.Unlock()
			return
		}
		segmenter, e := manager.Open(ctx, j.config.ChannelID)
		if e != nil {
			manager.Close()
			j.mu.Lock()
			j.status.HLS = "segmenter_unavailable"
			j.mu.Unlock()
			return
		}
		if ctx.Err() != nil {
			segmenter.Close()
			manager.Close()
			return
		}
		j.mu.Lock()
		j.hlsManager = manager
		j.hlsOutput = segmenter
		j.status.HLS = "running"
		j.mu.Unlock()
	}()
}
