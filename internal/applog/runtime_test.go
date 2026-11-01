package applog

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gcode/internal/storage"
)

func TestStartRuntimeMirrorsAndReleasesLogBeforeRestart(t *testing.T) {
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var console bytes.Buffer
	previous := log.Writer()
	cleanup, err := StartRuntime(store, &console)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	log.Print("runtime diagnostic")
	cleanup()
	cleanup()
	if log.Writer() != previous {
		t.Fatal("standard logger output was not restored")
	}
	file, err := os.ReadFile(filepath.Join(store.Directory(), "logs", "gcode.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(file, console.Bytes()) || !bytes.Contains(file, []byte("runtime diagnostic")) {
		t.Fatalf("runtime log %q differs from console %q", file, console.Bytes())
	}
	replacement, err := storage.OpenRuntimeLog(store)
	if err != nil {
		t.Fatalf("replacement process could not acquire log lock: %v", err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeOutputReportsFileFailuresAndKeepsConsole(t *testing.T) {
	diskErr := errors.New("test disk unavailable")
	closeErr := errors.New("test disk close failed")
	file := &failingFile{writeErr: diskErr, closeErr: closeErr}
	var console bytes.Buffer
	output := &runtimeOutput{console: &console, file: file}
	for range 2 {
		if n, err := output.Write([]byte("visible diagnostic\n")); n != 19 || !errors.Is(err, diskErr) {
			t.Fatalf("write = %d, %v", n, err)
		}
	}
	if err := output.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("close = %v", err)
	}
	if err := output.Close(); err != nil || file.closed != 1 {
		t.Fatalf("repeated close = %v; closed %d times", err, file.closed)
	}
	if _, err := output.Write([]byte("late")); !errors.Is(err, fs.ErrClosed) {
		t.Fatalf("write after close = %v", err)
	}
	for _, want := range []string{"visible diagnostic", "runtime log write failed: test disk unavailable", "runtime log close failed: test disk close failed"} {
		if !strings.Contains(console.String(), want) {
			t.Fatalf("console missing %q: %s", want, console.String())
		}
	}
}

func TestRuntimeOutputStillWritesFileWhenConsoleFails(t *testing.T) {
	file := &failingFile{}
	consoleErr := errors.New("test console unavailable")
	output := &runtimeOutput{console: failingConsole{consoleErr}, file: file}
	if _, err := output.Write([]byte("persist this")); !errors.Is(err, consoleErr) {
		t.Fatalf("write = %v", err)
	}
	if file.String() != "persist this" {
		t.Fatalf("file = %q", file.String())
	}
}

func TestRuntimeOutputReportsShortDiskWrite(t *testing.T) {
	var console bytes.Buffer
	output := &runtimeOutput{console: &console, file: &failingFile{short: true}}
	if _, err := output.Write([]byte("message")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write = %v", err)
	}
	if !strings.Contains(console.String(), "runtime log write failed: short write") {
		t.Fatalf("console = %q", console.String())
	}
}

type failingFile struct {
	bytes.Buffer
	writeErr error
	closeErr error
	closed   int
	short    bool
}

func (f *failingFile) Write(data []byte) (int, error) {
	if f.short {
		return 0, nil
	}
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.Buffer.Write(data)
}
func (f *failingFile) Close() error {
	f.closed++
	return f.closeErr
}

type failingConsole struct{ err error }

func (w failingConsole) Write([]byte) (int, error) { return 0, w.err }
