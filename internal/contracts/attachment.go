package contracts

import (
	"context"
	"slices"
)

const (
	MaxAttachmentBytes          = 24 * 1024 * 1024
	MaxAttachmentCount          = 20
	MaxAttachmentBatchBytes     = 96 * 1024 * 1024
	MaxAttachmentImageDimension = 16_384
	MaxAttachmentImagePixels    = 32_000_000
)

// ClipboardAttachment is encoded file/image content from a native clipboard.
type ClipboardAttachment struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Data []byte `json:"data"`
}

type AttachmentImportRequest struct {
	SessionID string               `json:"sessionId,omitempty"`
	Paths     []string             `json:"paths,omitempty"`
	Clipboard *ClipboardAttachment `json:"clipboard,omitempty"`
	// Transfer simulates a cancellable backend transfer after local ingestion.
	Transfer bool `json:"transfer"`
}

// Clone freezes native input before dispatch to a background importer.
func (r AttachmentImportRequest) Clone() AttachmentImportRequest {
	r.Paths = slices.Clone(r.Paths)
	if r.Clipboard != nil {
		v := *r.Clipboard
		v.Data = slices.Clone(v.Data)
		r.Clipboard = &v
	}
	return r
}

// AttachmentImporter stages user-selected paths or native clipboard bytes in
// the backend's independent managed store. It never sends data to a model.
type AttachmentImporter interface {
	ImportAttachments(context.Context, AttachmentImportRequest) ([]Attachment, error)
}
