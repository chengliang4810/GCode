package engine

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gcode/internal/contracts"
)

type browseTestReader struct {
	read  func(int) ([]fs.DirEntry, error)
	close func() error
}

func (r browseTestReader) ReadDir(n int) ([]fs.DirEntry, error) { return r.read(n) }
func (r browseTestReader) Close() error {
	if r.close != nil {
		return r.close()
	}
	return nil
}

func testDirectoryBrowser() *directoryBrowser {
	return &directoryBrowser{
		permits: make(chan struct{}, 1), timeout: 100 * time.Millisecond,
		home: func() (string, error) { return "/home", nil },
		abs:  func(p string) (string, error) { return p, nil },
		open: func(string) (directoryReader, error) {
			return browseTestReader{read: func(int) ([]fs.DirEntry, error) { return nil, io.EOF }}, nil
		},
		roots: func() []string { return []string{"/"} },
	}
}

func waitBrowseResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("directory caller did not return while OS work was blocked")
		return nil
	}
}

func waitBrowsePermit(t *testing.T, b *directoryBrowser) {
	t.Helper()
	select {
	case b.permits <- struct{}{}:
		<-b.permits
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not release its permit after OS work returned")
	}
}

func TestDirectoryBrowseTimeoutCoversEveryOSStage(t *testing.T) {
	for _, stage := range []string{"home", "abs", "open", "read", "close", "location-home", "volumes"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			b := testDirectoryBrowser()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			block := func() { close(entered); <-release }
			switch stage {
			case "home", "location-home":
				b.home = func() (string, error) { block(); return "/home", nil }
			case "abs":
				b.abs = func(p string) (string, error) { block(); return p, nil }
			case "open":
				open := b.open
				b.open = func(p string) (directoryReader, error) { block(); return open(p) }
			case "read":
				b.open = func(string) (directoryReader, error) {
					return browseTestReader{read: func(int) ([]fs.DirEntry, error) { block(); return nil, io.EOF }}, nil
				}
			case "close":
				b.open = func(string) (directoryReader, error) {
					return browseTestReader{read: func(int) ([]fs.DirEntry, error) { return nil, io.EOF }, close: func() error { block(); return nil }}, nil
				}
			case "volumes":
				b.roots = func() []string { block(); return []string{"/"} }
			}
			result := make(chan error, 1)
			go func() {
				var err error
				if stage == "volumes" || stage == "location-home" {
					_, err = b.listDirectoryLocations(context.Background(), contracts.DirectoryRequest{})
				} else {
					_, err = b.listDirectories(context.Background(), contracts.DirectoryRequest{})
				}
				result <- err
			}()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("OS stage not reached")
			}
			if err := waitBrowseResult(t, result); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("timeout error = %v", err)
			}
			// Neither query may admit more work while the abandoned OS call
			// still owns the only permit. Repeat to detect timeout leaks.
			for range 100 {
				if _, err := b.listDirectories(context.Background(), contracts.DirectoryRequest{}); !errors.Is(err, contracts.ErrDirectoryBrowseBusy) {
					t.Fatalf("directory saturation error = %v", err)
				}
				if _, err := b.listDirectoryLocations(context.Background(), contracts.DirectoryRequest{}); !errors.Is(err, contracts.ErrDirectoryBrowseBusy) {
					t.Fatalf("location saturation error = %v", err)
				}
			}
			unblock()
			waitBrowsePermit(t, b)
		})
	}
}

func TestDirectoryBrowseCancellationAndValidation(t *testing.T) {
	for _, locations := range []bool{false, true} {
		name := "directories"
		if locations {
			name = "locations"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := testDirectoryBrowser()
			b.timeout = time.Hour
			var calls atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			b.home = func() (string, error) { calls.Add(1); close(entered); <-release; return "/home", nil }
			query := func(ctx context.Context, device string) error {
				if locations {
					_, err := b.listDirectoryLocations(ctx, contracts.DirectoryRequest{DeviceID: device})
					return err
				}
				_, err := b.listDirectories(ctx, contracts.DirectoryRequest{DeviceID: device})
				return err
			}
			if err := query(context.Background(), "unknown"); err == nil {
				t.Fatal("unknown device accepted")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := query(ctx, "local"); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
				t.Fatalf("pre-canceled query performed IO: %v, calls %d", err, calls.Load())
			}
			ctx, cancel = context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- query(ctx, "local") }()
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("home lookup not started")
			}
			cancel()
			if err := waitBrowseResult(t, result); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel error = %v", err)
			}
			if err := query(context.Background(), "local"); !errors.Is(err, contracts.ErrDirectoryBrowseBusy) || calls.Load() != 1 {
				t.Fatalf("cancellation released real worker early: %v, calls %d", err, calls.Load())
			}
			unblock()
			waitBrowsePermit(t, b)
		})
	}
}

func TestEngineDirectoryQueriesWithoutViewLock(t *testing.T) {
	// Both zero-value engines browse real host directories without consulting
	// engine state or acquiring the view lock, even while it is held elsewhere.
	path := t.TempDir()
	for range 2 {
		core := &Engine{}
		core.mu.Lock()
		result := make(chan error, 1)
		go func() {
			_, err := core.ListDirectories(context.Background(), contracts.DirectoryRequest{DeviceID: "local", Path: path})
			if err == nil {
				_, err = core.ListDirectoryLocations(context.Background(), contracts.DirectoryRequest{DeviceID: "local"})
			}
			result <- err
		}()
		err := waitBrowseResult(t, result)
		core.mu.Unlock()
		if err != nil {
			t.Fatalf("engine browsing while view lock held: %v", err)
		}
	}
}

func TestDirectoryBrowseTotalWorkerLimit(t *testing.T) {
	b := testDirectoryBrowser()
	b.permits = make(chan struct{}, directoryBrowseWorkers)
	b.timeout = time.Hour
	entered := make(chan struct{}, directoryBrowseWorkers)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int32
	b.home = func() (string, error) {
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return "/home", nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, directoryBrowseWorkers)
	for i := range directoryBrowseWorkers {
		go func() {
			if i%2 == 0 {
				_, err := b.listDirectories(ctx, contracts.DirectoryRequest{})
				result <- err
			} else {
				_, err := b.listDirectoryLocations(ctx, contracts.DirectoryRequest{})
				result <- err
			}
		}()
	}
	for range directoryBrowseWorkers {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("bounded workers were not all admitted")
		}
	}
	cancel()
	for range directoryBrowseWorkers {
		if err := waitBrowseResult(t, result); !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled caller: %v", err)
		}
	}
	for range 100 {
		if _, err := b.listDirectories(context.Background(), contracts.DirectoryRequest{}); !errors.Is(err, contracts.ErrDirectoryBrowseBusy) {
			t.Fatalf("saturated directories: %v", err)
		}
		if _, err := b.listDirectoryLocations(context.Background(), contracts.DirectoryRequest{}); !errors.Is(err, contracts.ErrDirectoryBrowseBusy) {
			t.Fatalf("saturated locations: %v", err)
		}
	}
	if calls.Load() != directoryBrowseWorkers || len(b.permits) != directoryBrowseWorkers {
		t.Fatalf("worker bound bypassed or permits released early: calls=%d, permits=%d", calls.Load(), len(b.permits))
	}
	unblock()
	waitBrowsePermit(t, b)
}

// browseEntry supplies directory metadata without touching any real filesystem.
type browseEntry struct {
	name string
	dir  bool
}

func (e browseEntry) Name() string { return e.name }
func (e browseEntry) IsDir() bool  { return e.dir }
func (e browseEntry) Type() fs.FileMode {
	if e.dir {
		return fs.ModeDir
	}
	return 0
}
func (e browseEntry) Info() (fs.FileInfo, error) { return nil, fs.ErrInvalid }

func browseEntryPath(name string) string { return filepath.ToSlash(filepath.Join("/home", name)) }
