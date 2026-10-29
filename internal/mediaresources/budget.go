// Package mediaresources bounds child media processes independently of channel
// count. The installed service has a shared task and memory budget.
package mediaresources

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

var ErrBusy = errors.New("media_resources_busy")
var ErrTranscodeBusy = fmt.Errorf("transcode_capacity: %w", ErrBusy)
var poolMu sync.Mutex

var inputs = make(chan struct{}, 2)
var outputs = make(chan struct{}, 2)

func acquire(pool chan struct{}) (func(), bool) {
	select {
	case pool <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-pool }) }, true
	default:
		return nil, false
	}
}

func Input() (func(), bool) { poolMu.Lock(); defer poolMu.Unlock(); return acquire(inputs) }

// Output reserves validation and the subsequent segmenter as one operation.
func Output() (func(), bool) { poolMu.Lock(); defer poolMu.Unlock(); return acquire(outputs) }

// ConfigureLimits is called at startup before media sources are opened.
func ConfigureLimits(inputLimit, outputLimit int) error {
	poolMu.Lock()
	defer poolMu.Unlock()
	if inputLimit < 1 || inputLimit > 512 || outputLimit < 1 || outputLimit > 128 {
		return errors.New("invalid media capacity")
	}
	if cap(inputs) == inputLimit && cap(outputs) == outputLimit {
		return nil
	}
	if len(inputs) != 0 || len(outputs) != 0 {
		return ErrBusy
	}
	inputs = make(chan struct{}, inputLimit)
	outputs = make(chan struct{}, outputLimit)
	return nil
}

func Configure(cmd *exec.Cmd) {
	cmd.Args = append([]string{cmd.Args[0], "-threads", "1"}, cmd.Args[1:]...)
	configureEnvironment(cmd)
}

// Encoding must keep pace with live HD video. Bound decoder/filter parallelism
// independently of the host CPU count; copy receivers keep their single-thread budget.
func ConfigureEncoding(cmd *exec.Cmd) {
	cmd.Args = append([]string{cmd.Args[0], "-threads", "2", "-filter_threads", "2", "-filter_complex_threads", "2"}, cmd.Args[1:]...)
	configureEnvironment(cmd)
}

func configureEnvironment(cmd *exec.Cmd) {
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(env, "OMP_NUM_THREADS=1", "OPENBLAS_NUM_THREADS=1", "MKL_NUM_THREADS=1")
}
