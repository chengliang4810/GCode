package contracts

import "context"

// BackgroundAdjustment describes the focal point and cover zoom of a local image.
type BackgroundAdjustment struct {
	FocalX float32 `json:"focalX"`
	FocalY float32 `json:"focalY"`
	Zoom   float32 `json:"zoom"`
}

// ResourceService reads managed application resources without exposing UI objects.
// The returned bytes belong to the caller. References cannot read arbitrary files.
type ResourceService interface {
	ReadResource(context.Context, string) ([]byte, error)
}

type ModelPickerSettings struct {
	ProviderID string                    `json:"providerId"`
	Options    map[string]string         `json:"options"`
	Selections map[string]ModelSelection `json:"selections"`
}

// ModelSelection remembers each provider's last model and its effort.
type ModelSelection struct {
	ModelID   string `json:"modelId"`
	Reasoning string `json:"reasoning"`
}
