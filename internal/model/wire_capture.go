package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type wireCapture struct {
	dir      string
	file     *os.File
	body     io.ReadCloser
	complete bool
	readErr  error
	writeErr error
}

func startWireCapture(parent, id string, attempt int, request []byte) (*wireCapture, error) {
	if !filepath.IsAbs(parent) {
		return nil, errors.New("capture directory must be an absolute path")
	}
	if err := os.Mkdir(parent, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return nil, errors.New("capture directory must be a private directory (0700)")
	}
	dir, err := os.MkdirTemp(parent, fmt.Sprintf("call-%s-%03d-", id, attempt))
	if err != nil {
		return nil, err
	}
	if err := protectWireCaptureDirectory(dir); err != nil {
		_ = os.Remove(dir)
		return nil, fmt.Errorf("protect capture directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "request.json"), request, 0600); err != nil {
		return nil, err
	}
	return &wireCapture{dir: dir}, nil
}

func (c *wireCapture) wrap(body io.ReadCloser) io.ReadCloser {
	c.body = body
	c.file, c.writeErr = os.OpenFile(filepath.Join(c.dir, "response.body"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	return c
}

func (c *wireCapture) Read(p []byte) (int, error) {
	n, err := c.body.Read(p)
	if n > 0 && c.writeErr == nil {
		_, c.writeErr = c.file.Write(p[:n])
	}
	if errors.Is(err, io.EOF) {
		c.complete = true
	} else if err != nil {
		c.readErr = err
	}
	return n, err
}

func (c *wireCapture) Close() error { return c.body.Close() }

func (c *wireCapture) drain() {
	if c.complete || c.readErr != nil || c.writeErr != nil {
		return
	}
	// Parsing may stop before EOF, including on the malformed event that
	// prompted the capture. Do not let an open stream stall the caller.
	timer := time.AfterFunc(3*time.Second, func() { _ = c.body.Close() })
	defer timer.Stop()
	_, _ = io.Copy(io.Discard, c)
}

func (c *wireCapture) finish(status int, requestID string) error {
	if c.file != nil {
		c.writeErr = errors.Join(c.writeErr, c.file.Close())
	}
	meta, err := json.Marshal(struct {
		HTTPStatus       int    `json:"httpStatus"`
		RequestID        string `json:"providerRequestId,omitempty"`
		ResponseComplete bool   `json:"responseComplete"`
	}{status, requestID, c.complete && c.writeErr == nil})
	if err == nil {
		err = os.WriteFile(filepath.Join(c.dir, "metadata.json"), meta, 0600)
	}
	return errors.Join(c.writeErr, err)
}
