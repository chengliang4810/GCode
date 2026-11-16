package pii

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestPrivacyCacheRequiresUserDirectory(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("LOCALAPPDATA", "")
	if err := PrepareCache(); err == nil {
		t.Fatal("model cache fell back despite missing user cache directory")
	}
}

func TestPrivacyCacheRejectsLinkedApplicationDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "cache"))
	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(base, "GCode")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := PrepareCache(); err == nil {
		t.Fatal("linked application cache was accepted")
	}
	if _, err := os.Stat(filepath.Join(target, "privacy-models")); !os.IsNotExist(err) {
		t.Fatal("rejected cache was written through the link")
	}
}

func TestPrivacyDownloadRejectsLinkedPartWithoutOverwritingTarget(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "original")
	if err := os.WriteFile(target, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "model")
	if err := os.Symlink(target, path+".part"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	server := httptest.NewServer(&files{m: map[string][]byte{"/m": []byte("new content")}})
	defer server.Close()
	var done atomic.Int64
	if err := fetch(context.Background(), server.URL+"/m", path, 11, sum([]byte("new content")), &done); err == nil {
		t.Fatal("linked download was accepted")
	}
	bytes, err := os.ReadFile(target)
	if err != nil || string(bytes) != "keep me" {
		t.Fatalf("linked download overwrote another file: %q %v", bytes, err)
	}
	if file, err := openRegular(path+".part", os.O_WRONLY|os.O_TRUNC, 0600); err == nil {
		file.Close()
		t.Fatal("regular file boundary followed a symlink")
	}
	bytes, _ = os.ReadFile(target)
	if string(bytes) != "keep me" {
		t.Fatal("file boundary truncated a linked target")
	}
}
