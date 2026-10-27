package main

import (
	"context"
	"flag"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"syscall"

	"kore-edge/internal/app"
	"kore-edge/internal/config"
	"kore-edge/internal/operations"
	"kore-edge/internal/stream"
)

func main() {
	// Keep the default runtime within the installer service's task/memory limits.
	// Explicit operator runtime settings remain authoritative.
	if os.Getenv("GOMAXPROCS") == "" && runtime.GOMAXPROCS(0) > 2 {
		runtime.GOMAXPROCS(2)
	}
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(128 << 20)
	}
	net.DefaultResolver.PreferGo = true
	path := flag.String("config", "", "required path to reviewed laboratory YAML configuration")
	releases := flag.String("release-dir", "", "private selected release directory; exclusive with -config")
	synthetic := flag.Bool("synthetic-ts", false, "emit paced synthetic MPEG-TS null packets to stdout (no network)")
	flag.Parse()
	if *synthetic {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if stream.Generate(ctx, os.Stdout) != nil && ctx.Err() == nil {
			os.Exit(1)
		}
		return
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if (*path == "") == (*releases == "") {
		slog.Error("provide exactly one of -config or -release-dir")
		os.Exit(1)
	}
	var c config.Config
	var err error
	if *releases != "" {
		var executable string
		executable, err = os.Executable()
		if err == nil {
			c, err = operations.LoadSelected(*releases, executable)
		}
	} else {
		c, err = config.Load(*path)
	}
	if err != nil {
		slog.Error("configuration rejected", "reason", err.Error())
		os.Exit(1)
	}
	var store config.Store
	if err := store.Apply(c); err != nil {
		slog.Error("configuration rejected")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	slog.Info("starting core API", "mode", c.Mode)
	if err := app.Run(ctx, c); err != nil {
		slog.Error("core API failed")
		os.Exit(1)
	}
	slog.Info("core API stopped")
}
