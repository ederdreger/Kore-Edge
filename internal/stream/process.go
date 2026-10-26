package stream

import (
	"context"
	"errors"
	"io"
	"kore-edge/internal/hls"
	"os"
	"os/exec"
	"time"
)

// Process exposes only a pipe and lifecycle operations, never raw stderr/logs.
type Process struct {
	Source string
	Output io.ReadCloser
	Wait   func() error
	Kill   func() error
}
type Factory func(context.Context) (*Process, error)

func commandFactory(path string, args ...string) Factory {
	return func(ctx context.Context) (*Process, error) {
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.WaitDelay = time.Second
		stdout, e := cmd.StdoutPipe()
		if e != nil {
			return nil, errors.New("source unavailable")
		}
		if e = cmd.Start(); e != nil {
			stdout.Close()
			return nil, errors.New("source unavailable")
		}
		return &Process{Output: stdout, Wait: cmd.Wait, Kill: cmd.Process.Kill}, nil
	}
}

func SyntheticFactory() (Factory, error) {
	path, e := os.Executable()
	if e != nil {
		return nil, errors.New("source unavailable")
	}
	return commandFactory(path, "-synthetic-ts"), nil
}

func AVFactory(path string) Factory {
	return func(ctx context.Context) (*Process, error) {
		p, e := commandFactory(path, hls.SyntheticArgs(true)...)(ctx)
		if e == nil {
			p.Source = "synthetic:ffmpeg-h264-aac"
		}
		return p, e
	}
}
