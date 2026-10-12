package pty

import (
	"bytes"
	"runtime"
	"testing"
	"time"
)

func TestUnicodeQA(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	p, err := Start(Options{Path: "/bin/sh", Args: []string{"sh", "-i"}, Env: []string{"TERM=dumb", "LANG=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8", "PS1=", "ENV="}})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	command := "printf 'RAW=中文|世界|😀|👍🏽|👨‍👩‍👧|é́|RAW_DONE\\n'\r"
	output := make(chan []byte, 8)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				output <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()
	if _, err := p.Write([]byte(command)); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !bytes.Contains(out.Bytes(), []byte("RAW_DONE")) {
		select {
		case chunk := <-output:
			out.Write(chunk)
		case <-deadline.C:
			t.Fatalf("timed out waiting for shell output: raw=%q bytes=% x", out.String(), out.Bytes())
		}
	}
	if !bytes.Contains(out.Bytes(), []byte("RAW=中文|世界|😀|👍🏽|👨‍👩‍👧|é́|RAW_DONE")) {
		t.Fatalf("raw output lost UTF-8: %q bytes=% x", out.String(), out.Bytes())
	}
	t.Logf("raw_utf8_command_bytes=% x", []byte(command))
	t.Logf("raw_utf8_output_valid=%t", bytes.Contains(out.Bytes(), []byte("RAW=中文|世界|😀|👍🏽|👨‍👩‍👧|é́|RAW_DONE")))
}
