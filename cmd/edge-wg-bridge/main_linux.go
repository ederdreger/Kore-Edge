//go:build linux

// This is a separate, explicitly installed privileged delegate, never the API.
package main

import (
	"context"
	"errors"
	"flag"
	"io"
	"kore-edge/internal/security"
	"kore-edge/internal/wgbridge"
	"kore-edge/internal/wireguard"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type bounded struct {
	data []byte
	mu   sync.Mutex
}

func (b *bounded) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.data)+len(p) > 1024*1024 {
		return 0, errors.New("output_limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

type uidKey struct{}

func run() error {
	role := flag.String("role", "", "edge or matrix")
	source := flag.String("application-state", "", "fixed private application state")
	state := flag.String("state-directory", "", "root-owned journal directory")
	socket := flag.String("socket", "", "root-owned UNIX socket path")
	uid := flag.Int("app-uid", -1, "authorized application UID")
	gid := flag.Int("app-gid", -1, "authorized socket group")
	ip := flag.String("ip-bin", "/usr/sbin/ip", "trusted ip binary")
	wg := flag.String("wg-bin", "/usr/bin/wg", "trusted wg binary")
	ss := flag.String("ss-bin", "/usr/bin/ss", "trusted ss binary")
	flag.Parse()
	if os.Geteuid() != 0 || (*role != "edge" && *role != "matrix") || *uid <= 0 || *gid <= 0 || !filepath.IsAbs(*source) || !filepath.IsAbs(*state) || !filepath.IsAbs(*socket) {
		return wgbridge.ErrUnavailable
	}
	if security.PrivateDirectory(*state, true) != nil {
		return wgbridge.ErrUnavailable
	}
	owner, e := os.Lstat(*state)
	if e != nil || owner.Sys().(*syscall.Stat_t).Uid != 0 {
		return wgbridge.ErrUnavailable
	}
	lock, e := os.OpenFile(filepath.Join(*state, "bridge.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return wgbridge.ErrUnavailable
	}
	binaries := map[string]string{"ip": *ip, "wg": *wg, "ss": *ss}
	for name, binary := range binaries {
		resolved, e := filepath.EvalSymlinks(binary)
		if e != nil {
			return e
		}
		info, e := os.Stat(resolved)
		if e != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Sys().(*syscall.Stat_t).Uid != 0 {
			return wgbridge.ErrUnavailable
		}
		binaries[name] = resolved
	}
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		binary, ok := binaries[name]
		if !ok {
			return nil, wgbridge.ErrUnavailable
		}
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, args...)
		command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
		out := &bounded{}
		command.Stdout = out
		command.Stderr = io.Discard
		if command.Run() != nil {
			return nil, wgbridge.ErrUnavailable
		}
		return out.data, nil
	}
	load := func() (wireguard.ManagedConfig, error) {
		info, e := os.Lstat(*source)
		if e != nil || !info.Mode().IsRegular() || info.Sys().(*syscall.Stat_t).Uid != uint32(*uid) {
			return wireguard.ManagedConfig{}, wgbridge.ErrUnavailable
		}
		return wireguard.ReadManaged(*source, *role)
	}
	engine, e := wgbridge.New(*state, load, runner)
	if e != nil {
		return e
	}
	parent, e := os.Lstat(filepath.Dir(*socket))
	if e != nil || !parent.IsDir() || parent.Mode()&os.ModeSymlink != 0 || parent.Mode().Perm()&0022 != 0 || parent.Sys().(*syscall.Stat_t).Uid != 0 {
		return wgbridge.ErrUnavailable
	}
	if info, e := os.Lstat(*socket); e == nil {
		if info.Mode()&os.ModeSocket == 0 || info.Sys().(*syscall.Stat_t).Uid != 0 {
			return wgbridge.ErrUnavailable
		}
		if os.Remove(*socket) != nil {
			return wgbridge.ErrUnavailable
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	listener, e := net.Listen("unix", *socket)
	if e != nil {
		return e
	}
	defer listener.Close()
	defer os.Remove(*socket)
	if os.Chown(*socket, 0, *gid) != nil || os.Chmod(*socket, 0660) != nil {
		return wgbridge.ErrUnavailable
	}
	handler := wgbridge.Handler(engine)
	server := &http.Server{ReadHeaderTimeout: time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096, ErrorLog: log.New(io.Discard, "", 0), ConnContext: func(ctx context.Context, c net.Conn) context.Context {
		peer := -1
		if conn, ok := c.(*net.UnixConn); ok {
			raw, e := conn.SyscallConn()
			if e == nil {
				_ = raw.Control(func(fd uintptr) {
					cred, e := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
					if e == nil {
						peer = int(cred.Uid)
					}
				})
			}
		}
		return context.WithValue(ctx, uidKey{}, peer)
	}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peer, ok := r.Context().Value(uidKey{}).(int)
		if !ok || (peer != 0 && peer != *uid) {
			w.WriteHeader(403)
			return
		}
		handler.ServeHTTP(w, r)
	})}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	e = server.Serve(listener)
	if e == http.ErrServerClosed {
		<-shutdownDone
		wait, cancel := context.WithTimeout(context.Background(), 55*time.Second)
		defer cancel()
		return engine.Wait(wait)
	}
	return e
}
func main() {
	if run() != nil {
		_, _ = os.Stderr.WriteString("WireGuard bridge unavailable; inspect reviewed installation and permissions.\n")
		os.Exit(1)
	}
}
