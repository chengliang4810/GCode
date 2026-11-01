package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/storage"
)

func TestReadLinkedFileSourceOwnershipAndRealSave(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	path := filepath.Join(workspace, "子目录", "文件 空格.md")
	content := "# 真实内容\n\n源会话文件。"
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	refreshProjectFilesForTest(t, core)
	request := contracts.LinkedFileRequest{SessionID: session.ID, Path: "子目录/文件 空格.md"}
	result, err := core.ReadLinkedFile(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	if result.File.ReadOnly || !result.File.Loaded || result.File.Content != content ||
		result.File.ProjectID != session.ProjectID || result.AbsolutePath != filepath.ToSlash(canonicalPath) {
		t.Fatalf("linked project file = %+v", result)
	}
	absolute, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: path})
	if err != nil || absolute.File.ID != result.File.ID {
		t.Fatalf("absolute path should reuse project ID: %+v, %v", absolute, err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "file.save", ID: result.File.ID, Text: "保存通过原生核心", Version: result.File.Version}); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != "保存通过原生核心" {
		t.Fatalf("saved = %q, %v", saved, err)
	}
	otherDir := t.TempDir()
	otherPath := filepath.Join(otherDir, "外部 文件.md")
	if err := os.WriteFile(otherPath, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	external, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: otherPath})
	if err != nil || !external.File.ReadOnly || external.File.ProjectID != "" || external.File.Content != "outside" {
		t.Fatalf("external file = %+v, %v", external, err)
	}
	reopened, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: otherPath})
	if err != nil || reopened.File.ID != external.File.ID {
		t.Fatalf("unstable external ID: %+v, %v", reopened, err)
	}
	noSession, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{Path: path})
	if err != nil || !noSession.File.ReadOnly || noSession.File.ID == result.File.ID {
		t.Fatalf("unowned absolute file = %+v, %v", noSession, err)
	}
	otherHost, _, _ := linkedFileTestCore(t)
	otherHostFile, err := otherHost.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{Path: path})
	if err != nil || otherHostFile.File.ID == noSession.File.ID {
		t.Fatalf("different host reused external ID: %+v, %v", otherHostFile, err)
	}
	core.mu.Lock()
	second := newTestSession(t, core, "", "other")
	core.mu.Unlock()
	otherSession, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: second.ID, Path: otherPath})
	if err != nil || otherSession.File.ID == external.File.ID {
		t.Fatalf("different session reused external ID: %+v, %v", otherSession, err)
	}
	// Worktree/session roots are authoritative even inside the same project.
	core.mu.Lock()
	core.state.Sessions[core.sessionIndex(session.ID)].CWD = filepath.Dir(path)
	core.mu.Unlock()
	outsideCWD := filepath.Join(workspace, "root.txt")
	if err := os.WriteFile(outsideCWD, []byte("outside cwd"), 0600); err != nil {
		t.Fatal(err)
	}
	refreshProjectFilesForTest(t, core)
	rootFile, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: outsideCWD})
	if err != nil || !rootFile.File.ReadOnly {
		t.Fatalf("file outside source CWD must be read-only: %+v %v", rootFile, err)
	}
}

func TestReadLinkedFileValidatesFilesAndCancellation(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{name: "binary.bin", data: []byte{0xff, 0x01}},
		{name: "nul.txt", data: []byte("a\x00b")},
		{name: "fake.png", data: []byte("not an image")},
		{name: "fake.svg", data: []byte("<html/>")},
		{name: "multiple.svg", data: []byte("<svg/><svg/>")},
		{name: "doctype.svg", data: []byte("<!DOCTYPE svg><svg/>")},
	} {
		if err := os.WriteFile(filepath.Join(workspace, entry.name), entry.data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	large, err := os.Create(filepath.Join(workspace, "large.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := large.Truncate(linkedTextLimit + 1); err != nil {
		t.Fatal(err)
	}
	_ = large.Close()
	for _, path := range []string{"binary.bin", "nul.txt", "fake.png", "fake.svg", "multiple.svg", "doctype.svg", "large.txt", ".", "../escape.txt", "missing.txt"} {
		t.Run(path, func(t *testing.T) {
			if _, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: path}); err == nil {
				t.Fatalf("accepted invalid linked file %q", path)
			}
		})
	}
	for _, request := range []contracts.LinkedFileRequest{
		{Path: "relative.txt"},
		{SessionID: "missing", Path: workspace},
		{SessionID: session.ID, Path: ""},
		{SessionID: session.ID, Path: "bad\x00path"},
	} {
		if _, err := core.ReadLinkedFile(t.Context(), request); err == nil {
			t.Fatalf("accepted invalid request %+v", request)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := core.ReadLinkedFile(ctx, contracts.LinkedFileRequest{SessionID: session.ID, Path: "fake.png"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read = %v", err)
	}
}

func TestReadLinkedFilePublishesLoadedFileBeforeLaterDeltas(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	if err := os.WriteFile(filepath.Join(workspace, "loaded.txt"), []byte("complete linked contents"), 0600); err != nil {
		t.Fatal(err)
	}
	refreshProjectFilesForTest(t, core)
	events, err := core.Subscribe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initial := <-events
	if initial.Snapshot == nil || initial.Snapshot.Files[0].Loaded {
		t.Fatal("expected an unloaded initial file")
	}
	result, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: "loaded.txt"})
	if err != nil {
		t.Fatal(err)
	}
	var read contracts.Event
	select {
	case read = <-events:
	default:
		t.Fatal("linked file returned without publishing its authoritative file event")
	}
	if read.Kind != "file.read" || read.Snapshot == nil || read.Revision <= initial.Revision ||
		read.Snapshot.Revision != read.Revision || read.Snapshot.Files[0] != result.File {
		t.Fatalf("file event does not own the returned loaded buffer: %+v", read)
	}
	core.mu.Lock()
	core.publishLocked(contracts.Event{Kind: "message.delta", SessionID: session.ID, MessageID: "stream", BlockKind: "text", Text: "next"})
	core.mu.Unlock()
	delta := <-events
	if delta.Snapshot != nil || delta.Sequence <= read.Sequence || delta.Revision <= read.Revision {
		t.Fatalf("later message delta overtook the file projection: %+v", delta)
	}
	current, err := core.Snapshot(t.Context())
	if err != nil || current.Files[0] != result.File {
		t.Fatalf("later event lost loaded file: %+v %v", current.Files, err)
	}
}

func TestReadLinkedFileImagesUseManagedResources(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	var pngBytes bytes.Buffer
	bitmap := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	bitmap.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(&pngBytes, bitmap); err != nil {
		t.Fatal(err)
	}
	svg := []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg" width="12" height="12"><path d="M0 0L12 12"/></svg>`)
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{name: "测试 图片.png", data: pngBytes.Bytes()},
		{name: "图标.svg", data: svg},
	} {
		t.Run(entry.name, func(t *testing.T) {
			path := filepath.Join(workspace, entry.name)
			if err := os.WriteFile(path, entry.data, 0600); err != nil {
				t.Fatal(err)
			}
			result, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: entry.name})
			if err != nil {
				t.Fatal(err)
			}
			if result.File.Kind != "image" || !strings.HasSuffix(result.File.AssetRef, filepath.Ext(entry.name)) || result.File.Content != "" {
				t.Fatalf("image file = %+v", result.File)
			}
			data, err := core.ReadResource(t.Context(), result.File.AssetRef)
			if err != nil || !bytes.Equal(data, entry.data) {
				t.Fatalf("managed resource mismatch: %v", err)
			}
			before, err := os.ReadDir(filepath.Join(core.storage.Directory(), "uploads"))
			if err != nil {
				t.Fatal(err)
			}
			var workers sync.WaitGroup
			failures := make(chan error, 12)
			for range 12 {
				workers.Go(func() {
					repeated, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: entry.name})
					if err == nil && repeated.File.AssetRef != result.File.AssetRef {
						err = errors.New("repeated image read did not reuse the managed resource")
					}
					if err == nil {
						got, readErr := core.ReadResource(t.Context(), repeated.File.AssetRef)
						if readErr != nil || !bytes.Equal(got, entry.data) {
							err = fmt.Errorf("concurrent image resource is partial or unreadable: %v", readErr)
						}
					}
					if err != nil {
						failures <- err
					}
				})
			}
			workers.Wait()
			close(failures)
			for err := range failures {
				t.Error(err)
			}
			after, err := os.ReadDir(filepath.Join(core.storage.Directory(), "uploads"))
			if err != nil || len(after) != len(before) {
				t.Fatalf("repeated image reads grew uploads: %d -> %d, %v", len(before), len(after), err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := core.ReadLinkedFile(ctx, contracts.LinkedFileRequest{SessionID: session.ID, Path: entry.name}); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled repeated image read = %v", err)
			}
			retained, err := core.ReadResource(t.Context(), result.File.AssetRef)
			if err != nil || !bytes.Equal(retained, entry.data) {
				t.Fatalf("failed read deleted shared image resource: %v", err)
			}
		})
	}
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{name: "wrong.jpg", data: pngBytes.Bytes()},
		{name: "truncated.png", data: pngBytes.Bytes()[:33]},
	} {
		if err := os.WriteFile(filepath.Join(workspace, entry.name), entry.data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: entry.name}); err == nil {
			t.Fatalf("accepted invalid image %q", entry.name)
		}
	}
}

func TestReadLinkedFileRejectsRelativeSymlinkEscape(t *testing.T) {
	core, workspace, session := linkedFileTestCore(t)
	path := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(path, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, filepath.Join(workspace, "escape.txt")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: "escape.txt"}); err == nil {
		t.Fatal("relative symlink escaped source directory")
	}
	result, err := core.ReadLinkedFile(t.Context(), contracts.LinkedFileRequest{SessionID: session.ID, Path: filepath.Join(workspace, "escape.txt")})
	if err != nil || !result.File.ReadOnly || result.File.Content != "outside" {
		t.Fatalf("absolute external symlink should be read-only: %+v %v", result, err)
	}
}

func TestCacheLinkedImagePublishesCompleteResourceOnce(t *testing.T) {
	directory := t.TempDir()
	data := bytes.Repeat([]byte("immutable image payload"), 4000)
	var workers sync.WaitGroup
	release := make(chan struct{})
	failures := make(chan error, 12)
	keys := make(chan string, 12)
	for range 12 {
		workers.Go(func() {
			<-release
			key, err := cacheLinkedImage(t.Context(), directory, ".png", data)
			if err != nil {
				failures <- err
				return
			}
			keys <- key
			published, err := os.ReadFile(filepath.Join(directory, "uploads", key))
			if err != nil || !bytes.Equal(published, data) {
				failures <- fmt.Errorf("concurrent publication exposed partial bytes: %v", err)
			}
		})
	}
	close(release)
	workers.Wait()
	close(failures)
	close(keys)
	for err := range failures {
		t.Error(err)
	}
	first := ""
	for key := range keys {
		if first == "" {
			first = key
		}
		if key != first {
			t.Errorf("concurrent publication used distinct resource keys: %q and %q", first, key)
		}
	}
	entries, err := os.ReadDir(filepath.Join(directory, "uploads"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("concurrent publication left %d files, want 1: %v", len(entries), err)
	}
}

func linkedFileTestCore(t *testing.T) (*Engine, string, contracts.Session) {
	t.Helper()
	store, err := storage.Open(storage.Options{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	core, err := Open(Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = core.Close() })
	workspace := t.TempDir()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "project.create", ProjectCreate: &contracts.ProjectCreate{Path: workspace}}); err != nil {
		t.Fatal(err)
	}
	core.mu.Lock()
	session := newTestSession(t, core, core.state.Projects[0].ID, "source")
	core.mu.Unlock()
	return core, workspace, session
}
