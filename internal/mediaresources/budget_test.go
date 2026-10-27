package mediaresources

import (
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentAdmissionAndRelease(t *testing.T) {
	for _, acquire := range []func() (func(), bool){Input, Output} {
		var accepted atomic.Int32
		var wg sync.WaitGroup
		release := make(chan struct{})
		ready := make(chan struct{}, 32)
		for i := 0; i < 32; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				done, ok := acquire()
				ready <- struct{}{}
				if ok {
					accepted.Add(1)
					<-release
					done()
					done()
				}
			}()
		}
		for i := 0; i < 32; i++ {
			<-ready
		}
		close(release)
		wg.Wait()
		if accepted.Load() != 2 {
			t.Fatal("unbounded process admission", accepted.Load())
		}
		done, ok := acquire()
		if !ok {
			t.Fatal("reservation leaked")
		}
		done()
	}
}

func TestProcessThreadBudget(t *testing.T) {
	cmd := exec.Command("synthetic-tool", "-i", "pipe:0", "-c", "copy")
	Configure(cmd)
	if len(cmd.Args) < 4 || cmd.Args[1] != "-threads" || cmd.Args[2] != "1" {
		t.Fatal("unbounded codec threads")
	}
	if cmd.Args[5] != "-c" || cmd.Args[6] != "copy" {
		t.Fatal("remux behavior changed")
	}
}
