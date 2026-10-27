// Package mediaresources bounds child media processes independently of channel
// count. The installed service has a shared task and memory budget.
package mediaresources

import (
	"errors"
	"os"
	"os/exec"
	"sync"
)

var ErrBusy = errors.New("media_resources_busy")
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

func Input() (func(), bool) { return acquire(inputs) }

// Output reserves validation and the subsequent segmenter as one operation.
func Output() (func(), bool) { return acquire(outputs) }

func Configure(cmd *exec.Cmd) {
	cmd.Args = append([]string{cmd.Args[0], "-threads", "1"}, cmd.Args[1:]...)
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(env, "OMP_NUM_THREADS=1", "OPENBLAS_NUM_THREADS=1", "MKL_NUM_THREADS=1")
}
