//go:build darwin || linux

package pty

import (
	"io"
	"os"
	"testing"
	"time"
)

func TestPTYDeliversChildOutputBeforeInput(t *testing.T) {
	p, err := Start(Options{Path: "/bin/sh", Args: []string{"/bin/sh", "-c", "sleep 0.05; printf ready; read ignored"}, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = p.Kill()
		_ = p.Close()
		_, _ = p.Wait()
	})
	received := make(chan string, 1)
	go func() {
		b := make([]byte, len("ready"))
		n, e := io.ReadFull(p, b)
		if e != nil {
			received <- e.Error()
		} else {
			received <- string(b[:n])
		}
	}()
	select {
	case got := <-received:
		if got != "ready" {
			t.Fatalf("output %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child output did not reach master")
	}
}
