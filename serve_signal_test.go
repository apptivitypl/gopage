//go:build !js && !windows

package gopage

import (
	"syscall"
	"testing"
	"time"
)

func TestServeStopsOnATerminationSignal(t *testing.T) {
	app, address := listenApp(t)
	stopped := make(chan error, 1)
	go func() { stopped <- Serve(address, app) }()

	waitUp(t, address)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("kill: %v", err)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Serve: %v, want a clean stop", err)
		}
	case <-time.After(GraceTimeout + 5*time.Second):
		t.Fatal("Serve never returned after the signal")
	}
}
