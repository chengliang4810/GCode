package desktop

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type mediaReadService struct {
	contracts.Service
	reads []string
	data  []byte
}

func (s *mediaReadService) ReadResource(_ context.Context, ref string) ([]byte, error) {
	s.reads = append(s.reads, ref)
	return s.data, nil
}

func TestMediaPrefetchDoesNotDecodeUnopenedHistory(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	a := testApp(t, "default")
	service := &mediaReadService{Service: a.service, data: encoded.Bytes()}
	a.service = service
	prefix := "managed://" + t.TempDir()
	hidden := contracts.Attachment{MIME: "image/png", AssetRef: prefix + "/unopened-history"}
	shown := contracts.Attachment{MIME: "image/png", AssetRef: prefix + "/visible-history"}
	queued := contracts.Attachment{MIME: "image/png", AssetRef: prefix + "/queued-image"}
	snapshot := contracts.Snapshot{Attachments: []contracts.Attachment{hidden, shown, queued}}
	a.prefetchMedia(snapshotMediaRefs(snapshot))
	if len(service.reads) != 0 {
		t.Fatalf("global attachment index caused reads: %v", service.reads)
	}
	snapshot.Messages = map[string][]contracts.Message{"selected": {{Blocks: []contracts.Block{{Attachment: &shown}}}}}
	snapshot.Queue = []contracts.QueuedMessage{{Attachments: []contracts.Attachment{queued}}}
	a.prefetchMedia(snapshotMediaRefs(snapshot))
	if len(service.reads) != 2 || service.reads[0] != shown.AssetRef || service.reads[1] != queued.AssetRef {
		t.Fatalf("projected previews were not loaded: %v", service.reads)
	}
	if loadMedia(shown.AssetRef).bitmap == nil || loadMedia(queued.AssetRef).bitmap == nil {
		t.Fatal("projected images have no preview")
	}
	a.imageRef, a.imageOpen, a.imageZoom = shown.AssetRef, true, 1
	tt := ui.NewTester(a.View, 1320, 880)
	if err := tt.Click("Zoom in"); err != nil {
		t.Fatal(err)
	}
	if a.imageZoom != 1.25 {
		t.Fatal("image preview did not zoom")
	}
	if err := tt.Click("Reset zoom"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Close image"); err != nil {
		t.Fatal(err)
	}
}
