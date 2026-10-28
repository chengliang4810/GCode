package mock

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gcode/internal/contracts"
)

func TestAttachmentImportsUseIndependentResourcesAndCatalog(t *testing.T) {
	ctx := context.Background()
	statePath := filepath.Join(t.TempDir(), "replica", "state.json")
	s := newTestServer(t, Options{StatePath: statePath})
	path, original := writeTestBackground(t, t.TempDir(), "capture.png")
	textPath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(textPath, []byte("project notes"), 0600); err != nil {
		t.Fatal(err)
	}
	items, err := s.ImportAttachments(ctx, contracts.AttachmentImportRequest{Paths: []string{path, textPath}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].MIME != "image/png" || items[0].Width != 8 || items[0].Height != 4 || items[0].Name != "capture.png" || items[0].Size != int64(len(original)) || items[0].Status != "ready" || items[1].MIME != "text/plain" {
		t.Fatalf("incorrect attachment metadata: %+v", items)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	data, err := s.ReadResource(ctx, items[0].AssetRef)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("managed image depended on source", err)
	}
	data[0] ^= 0xff
	again, _ := s.ReadResource(ctx, items[0].AssetRef)
	if !bytes.Equal(again, original) {
		t.Fatal("resource read shared mutable bytes")
	}
	v, _ := s.Snapshot(ctx)
	if len(v.Attachments) != 2 || len(v.Appshots) != 0 {
		t.Fatal("import mutated Appshots instead of attachment catalog")
	}
	v.Attachments[0].Name = "changed"
	items[0].Name = "changed result"
	owned, _ := s.Snapshot(ctx)
	if owned.Attachments[0].Name != "capture.png" {
		t.Fatal("attachment catalog aliases returned values")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := newTestServer(t, Options{StatePath: statePath})
	data, err = restarted.ReadResource(ctx, owned.Attachments[0].AssetRef)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("persistent attachment resource was not recovered", err)
	}
	if _, err = restarted.ReadResource(ctx, "resource://managed/attachment-../../state.json"); !errors.Is(err, ErrNotFound) {
		t.Fatal("arbitrary resource read allowed")
	}
}

func TestClipboardImportFreezesBytesAndValidatesMIME(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	_, original := writeTestBackground(t, t.TempDir(), "input.png")
	req := contracts.AttachmentImportRequest{Clipboard: &contracts.ClipboardAttachment{Name: "image", MIME: "image/png", Data: slices.Clone(original)}}
	frozen := req.Clone()
	req.Clipboard.Data[0] ^= 0xff
	items, err := s.ImportAttachments(ctx, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Name != "image.png" || items[0].MIME != "image/png" {
		t.Fatal("clipboard image extension/type incorrect")
	}
	frozen.Clipboard.Data[0] ^= 0xff
	actual, _ := s.ReadResource(ctx, items[0].AssetRef)
	if !bytes.Equal(actual, original) {
		t.Fatal("backend retained clipboard input alias")
	}
	before, _ := s.Snapshot(ctx)
	_, err = s.ImportAttachments(ctx, contracts.AttachmentImportRequest{Clipboard: &contracts.ClipboardAttachment{MIME: "image/jpeg", Data: original}})
	if err == nil {
		t.Fatal("mismatched clipboard image type accepted")
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected clipboard content mutated catalog")
	}
}

func TestAttachmentImportValidationAndBatchAtomicity(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	path, png := writeTestBackground(t, t.TempDir(), "valid.png")
	wide := slices.Clone(png)
	binary.BigEndian.PutUint32(wide[16:20], contracts.MaxAttachmentImageDimension+1)
	binary.BigEndian.PutUint32(wide[29:33], crc32.ChecksumIEEE(wide[12:29]))
	largePixels := slices.Clone(png)
	binary.BigEndian.PutUint32(largePixels[16:20], 8192)
	binary.BigEndian.PutUint32(largePixels[20:24], 8192)
	binary.BigEndian.PutUint32(largePixels[29:33], crc32.ChecksumIEEE(largePixels[12:29]))
	oversized := filepath.Join(t.TempDir(), "oversized.bin")
	f, err := os.Create(oversized)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(contracts.MaxAttachmentBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	requests := []contracts.AttachmentImportRequest{
		{},
		{Paths: make([]string, contracts.MaxAttachmentCount+1)},
		{Paths: []string{path, filepath.Join(t.TempDir(), "missing")}},
		{Paths: []string{t.TempDir()}},
		{Paths: []string{oversized}},
		{Clipboard: &contracts.ClipboardAttachment{MIME: "image/png", Data: []byte("broken")}},
		{Clipboard: &contracts.ClipboardAttachment{MIME: "image/png", Data: wide}},
		{Clipboard: &contracts.ClipboardAttachment{MIME: "image/png", Data: largePixels}},
		{Clipboard: &contracts.ClipboardAttachment{MIME: "image/svg+xml", Data: []byte(`<svg width="99999" height="1"/>`)}},
	}
	before, _ := s.Snapshot(ctx)
	for i, request := range requests {
		if _, err = s.ImportAttachments(ctx, request); err == nil {
			t.Errorf("invalid import %d accepted", i)
		}
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed batch left a partial catalog")
	}
	if len(s.resources) != 0 {
		t.Fatal("failed import leaked in-memory resources")
	}
}

func TestSVGAttachmentMetadataAndTransferCancellation(t *testing.T) {
	s := newTestServer(t, Options{TokenInterval: time.Hour})
	ctx := context.Background()
	events, _ := s.Subscribe(ctx)
	<-events
	importCtx, cancel := context.WithCancel(ctx)
	items, err := s.ImportAttachments(importCtx, contracts.AttachmentImportRequest{Transfer: true, Clipboard: &contracts.ClipboardAttachment{Name: "diagram", MIME: "image/svg+xml", Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 40 20"><rect width="40" height="20"/></svg>`)}})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if items[0].Width != 40 || items[0].Height != 20 || items[0].Status != "transferring" || items[0].Name != "diagram.svg" {
		t.Fatal("SVG/transfer metadata incorrect")
	}
	cancel()
	await(t, events, func(v contracts.Snapshot) bool {
		return len(v.Attachments) == 1 && v.Attachments[0].Status == "cancelled"
	})
	if _, err = s.ReadResource(ctx, items[0].AssetRef); err != nil {
		t.Fatal("cancelled transfer discarded user-selected bytes", err)
	}
	cancelled, cancelBefore := context.WithCancel(ctx)
	cancelBefore()
	before, _ := s.Snapshot(ctx)
	if _, err = s.ImportAttachments(cancelled, contracts.AttachmentImportRequest{Paths: []string{"not-read"}}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled import started work", err)
	}
	after, _ := s.Snapshot(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("cancelled import changed state")
	}
}

func TestAttachmentTransferCompletesAndCloseSettlesPending(t *testing.T) {
	ctx := context.Background()
	s := newTestServer(t, Options{Instant: true})
	events, _ := s.Subscribe(ctx)
	<-events
	if _, err := s.ImportAttachments(ctx, contracts.AttachmentImportRequest{Transfer: true, Clipboard: &contracts.ClipboardAttachment{Name: "notes.txt", MIME: "text/plain", Data: []byte("notes")}}); err != nil {
		t.Fatal(err)
	}
	await(t, events, func(v contracts.Snapshot) bool { return len(v.Attachments) == 1 && v.Attachments[0].Status == "ready" })
	statePath := filepath.Join(t.TempDir(), "state.json")
	pending := newTestServer(t, Options{StatePath: statePath, TokenInterval: time.Hour})
	if _, err := pending.ImportAttachments(ctx, contracts.AttachmentImportRequest{Transfer: true, Clipboard: &contracts.ClipboardAttachment{Name: "notes.txt", MIME: "text/plain", Data: []byte("notes")}}); err != nil {
		t.Fatal(err)
	}
	if err := pending.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := newTestServer(t, Options{StatePath: statePath})
	state, _ := restarted.Snapshot(ctx)
	if state.Attachments[0].Status != "cancelled" {
		t.Fatal("restart retained a process-local transfer")
	}
	if _, err := pending.ImportAttachments(ctx, contracts.AttachmentImportRequest{Clipboard: &contracts.ClipboardAttachment{MIME: "text/plain", Data: []byte("notes")}}); !errors.Is(err, ErrClosed) {
		t.Fatal("closed importer accepted work", err)
	}
}

func TestAttachmentImportPersistenceFailureRetiresPreparedFiles(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	s := newTestServer(t, Options{StatePath: statePath})
	path, original := writeTestBackground(t, t.TempDir(), "capture.png")
	if err := os.Mkdir(statePath, 0700); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Snapshot(context.Background())
	if _, err := s.ImportAttachments(context.Background(), contracts.AttachmentImportRequest{Paths: []string{path}}); err == nil {
		t.Fatal("failed catalog persistence reported success")
	}
	after, _ := s.Snapshot(context.Background())
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed save exposed an uncommitted attachment")
	}
	files, err := os.ReadDir(filepath.Join(filepath.Dir(statePath), "attachments"))
	if err != nil || len(files) != 0 {
		t.Fatal("failed save retained prepared attachment files", err)
	}
	bytesOnDisk, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(bytesOnDisk, original) {
		t.Fatal("failed import modified the user's source", err)
	}
	if err = os.Remove(statePath); err != nil {
		t.Fatal(err)
	}
}
