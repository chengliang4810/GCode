package pii

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// dlState is a model's download under way, one at a time.
type dlState struct {
	mu     sync.Mutex
	cancel context.CancelFunc // nil when none is under way
	err    string             // the last one's, "" when it worked
	total  int64
	done   atomic.Int64
}

// dl is Privacy Filter's.
var dl dlState

// client honors the host environment proxy and bounds response headers.
var client = func() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyFromEnvironment
	t.ResponseHeaderTimeout = 60 * time.Second
	return &http.Client{Transport: t}
}()

// stallAfter is how long a download waits for its next byte.
var stallAfter = 60 * time.Second

// Download fetches pf-serve and the model, each checked against its
// sha256, in the background, and calls done once both are on disk. A
// download already under way is left to go on. What was fetched of the
// model before (a download stopped, magpie quit) is taken up where it
// stopped.
func Download(done func()) error {
	if !Supported() {
		return fmt.Errorf("the local masking model doesn't run on %s yet", target())
	}
	return dl.run(modelSize, install, done)
}

// run runs install in the background, unless a download is under way,
// and calls done once it worked.
func (d *dlState) run(total int64, install func(context.Context) error, done func()) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel, d.err, d.total = cancel, "", total
	d.done.Store(0)
	go func() {
		err := install(ctx)
		d.mu.Lock()
		d.cancel = nil
		if err != nil {
			d.err = err.Error()
			if ctx.Err() != nil {
				d.err = ""
			}
		}
		d.mu.Unlock()
		cancel()
		if err == nil && done != nil {
			done()
		}
	}()
	return nil
}

// Cancel stops the download under way, keeping what it fetched.
func Cancel() { dl.stop() }

func (d *dlState) stop() {
	d.mu.Lock()
	if d.cancel != nil {
		d.cancel()
	}
	d.mu.Unlock()
}

// fill puts d's state in s.
func (d *dlState) fill(s *Status) {
	d.mu.Lock()
	s.Downloading, s.Error = d.cancel != nil, d.err
	s.Done, s.Total = d.done.Load(), d.total
	d.mu.Unlock()
}

func install(ctx context.Context) error {
	if err := privateDirectory(dir()); err != nil {
		return err
	}
	if err := ensureServe(ctx); err != nil {
		return fmt.Errorf("downloading pf-serve: %w", err)
	}
	if regularModelFile(modelPath(), modelSize) {
		dl.done.Store(modelSize)
		return nil
	}
	if err := fetch(ctx, modelURL, modelPath(), modelSize, modelSHA, &dl.done); err != nil {
		return fmt.Errorf("downloading the model: %w", err)
	}
	return nil
}

// serveMu keeps install and upgrade from fetching serveTag's pf-serve at
// once.
var serveMu sync.Mutex

// ensureServe fetches serveTag's pf-serve unless it is on disk.
func ensureServe(ctx context.Context) error {
	serveMu.Lock()
	defer serveMu.Unlock()
	if regularModelFile(exeIn(serveDir(), "pf-serve"), -1) {
		return nil
	}
	return installServe(ctx)
}

// installServe fetches pf-serve's zip and puts what is in it in
// serveDir: each file by its name alone, so nothing lands outside it.
func installServe(ctx context.Context) error {
	z := filepath.Join(dir(), serveTag+"-"+target()+".zip")
	defer os.Remove(z)
	var n atomic.Int64
	if err := fetch(ctx, serveBase+serveTag+"/pf-serve-"+target()+".zip", z, -1, serveSHA[target()], &n); err != nil {
		return err
	}
	zr, err := zip.OpenReader(z)
	if err != nil {
		return err
	}
	defer zr.Close()
	tmp, err := os.MkdirTemp(dir(), ".privacy-runtime-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if err := unzipTo(f, filepath.Join(tmp, filepath.Base(f.Name))); err != nil {
			os.RemoveAll(tmp)
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, filepath.Base(exe("pf-serve")))); err != nil {
		os.RemoveAll(tmp)
		return fmt.Errorf("the zip has no %s", filepath.Base(exe("pf-serve")))
	}
	os.RemoveAll(serveDir())
	return os.Rename(tmp, serveDir())
}

func unzipTo(f *zip.File, path string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := openRegular(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Fetch is fetch for another package's pinned file (memembed's model):
// through magpie's proxy, taken up where a stopped one left off, and
// renamed into place only when its sha256 is want.
func Fetch(ctx context.Context, url, path string, size int64, want string, done *atomic.Int64) error {
	return fetch(ctx, url, path, size, want, done)
}

// fetch downloads url to path through path+".part", taking up a part
// already there with a Range request, and renames it to path only when
// its sha256 is want (and its size size, unless that is -1). done counts
// the bytes on disk as they come.
func fetch(ctx context.Context, url, path string, size int64, want string, done *atomic.Int64) error {
	if want == "" {
		return errors.New("no checksum pinned for " + url)
	}
	part := path + ".part"
	have := int64(0)
	if fi, err := os.Lstat(part); err == nil {
		if !fi.Mode().IsRegular() {
			return errors.New("privacy model download target must be a regular file")
		}
		have = fi.Size()
		if size >= 0 && have > size {
			os.Remove(part)
			have = 0
		}
	}
	if size < 0 || have < size {
		got, err := get(ctx, url, part, have, done)
		if err != nil {
			return err
		}
		have = got
	}
	done.Store(have)
	if size >= 0 && have != size {
		return fmt.Errorf("got %d bytes of %d", have, size)
	}
	f, err := openRegular(part, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		// a part taken up that wasn't this file's: the next try starts over
		os.Remove(part)
		return errors.New("the download's checksum doesn't match; try again")
	}
	return os.Rename(part, path)
}

// get appends what url has from byte from on to part, and says how long
// part is now. A server that answers the whole file has it written over.
func get(ctx context.Context, url, part string, from int64, done *atomic.Int64) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if from > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(from, 10)+"-")
	}
	// a mirror may serve it: what it serves is checked against the pin
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	switch {
	case res.StatusCode == http.StatusPartialContent && from > 0:
		flags |= os.O_APPEND
	case res.StatusCode == http.StatusOK:
		flags |= os.O_TRUNC
		from = 0
	default:
		return 0, fmt.Errorf("%s: %s", url, res.Status)
	}
	out, err := openRegular(part, flags, 0o600)
	if err != nil {
		return 0, err
	}
	n := from
	done.Store(n)
	var stalled atomic.Bool
	stall := time.AfterFunc(stallAfter, func() { stalled.Store(true); cancel() })
	defer stall.Stop()
	buf := make([]byte, 256<<10)
	for {
		k, rerr := res.Body.Read(buf)
		if k > 0 {
			stall.Reset(stallAfter)
			if _, err := out.Write(buf[:k]); err != nil {
				out.Close()
				return n, err
			}
			n += int64(k)
			done.Store(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			if stalled.Load() {
				return n, fmt.Errorf("no data for %s", stallAfter)
			}
			return n, rerr
		}
	}
	return n, out.Close()
}
