package assets

import (
	"embed"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Artwork and short event sounds from Cindy; licenses ship beside the assets.
//
//go:embed island/*
var islandFiles embed.FS
var islandImages sync.Map

func IslandMascot(name string) ui.ImageSource {
	if value, ok := islandImages.Load(name); ok {
		return value.(ui.ImageSource)
	}
	extension := ".png"
	if name == "pululu" {
		extension = ".svg"
	}
	data, err := islandFiles.ReadFile("island/agent-island-" + name + extension)
	if err != nil {
		return nil
	}
	image, err := ui.DecodeImage(data)
	if err != nil {
		return nil
	}
	value, _ := islandImages.LoadOrStore(name, image)
	return value.(ui.ImageSource)
}

func IslandSound(name string) ([]byte, error) { return islandFiles.ReadFile("island/" + name + ".wav") }
