//go:build linux

package vm

import (
	"sync"
	"testing"
	"time"
)

// probeCount returns the number of live probes registered in the HealthChecker.
// Tests use it to assert the observable effect of Start/Stop (a probe appears or
// disappears) instead of only checking that a call did not panic.
func probeCount(hc *HealthChecker) int {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	return len(hc.probes)
}

// waitOrTimeout waits for wg to reach zero within d, failing the test with a
// clear message otherwise. It turns a would-be hang (a goroutine that never
// stops) into an explicit, fast assertion rather than relying on the global
// `go test` timeout to eventually kill the run.
func waitOrTimeout(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("goroutine did not stop within %s", d)
	}
}
