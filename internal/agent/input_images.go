package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strings"

	"gcode/internal/model"
)

// Only new inputs are prepared. The committed history is a stable prefix,
// including across retries and restart; no permanent "already seen" cache exists.
func prepareInputImages(ctx context.Context, history []model.Message, message model.Message) (model.Message, error) {
	message.Images = slices.Clone(message.Images)
	for i, original := range message.Images {
		if len(original.Data) > base64.StdEncoding.EncodedLen(maxReadImageBytes) {
			return message, fmt.Errorf("attachment image %d exceeds 24 MiB", i+1)
		}
		data, err := base64.StdEncoding.DecodeString(original.Data)
		if err != nil {
			return message, fmt.Errorf("decode attachment image %d: %w", i+1, err)
		}
		output, err := prepareImage(ctx, data, fmt.Sprintf("attachment %d", i+1))
		if err != nil {
			return message, fmt.Errorf("prepare attachment image %d: %w", i+1, err)
		}
		message.Images[i] = output.Images[0]
		if original != output.Images[0] {
			message.Text += "\n" + output.Text
		}
	}
	return deduplicateImages(history, message), nil
}

// A reference is valid only while the actual bytes remain in current context.
// Reference-only messages never count as evidence that an image is available.
func deduplicateImages(history []model.Message, message model.Message) model.Message {
	if message.IsError || len(message.Images) == 0 {
		return message
	}
	seen := make(map[string]string)
	for _, prior := range history {
		if prior.IsError {
			continue
		}
		for _, picture := range prior.Images {
			key := imageHash(picture)
			if _, exists := seen[key]; exists {
				continue
			}
			location := "an earlier " + prior.Role + " message"
			if prior.Role == "tool" && prior.CallID != "" {
				location = "tool result " + prior.CallID
			}
			seen[key] = location
		}
	}
	images := make([]model.Image, 0, len(message.Images))
	message.ImageRefs = slices.Clone(message.ImageRefs)
	for _, picture := range message.Images {
		key := imageHash(picture)
		label := "Image " + key[:16]
		if location, exists := seen[key]; exists {
			message.Text += "\n" + label + ": identical image is still available in " + location + "."
			if !slices.Contains(message.ImageRefs, key) {
				message.ImageRefs = append(message.ImageRefs, key)
			}
			continue
		}
		images = append(images, picture)
		seen[key] = "this message"
		if !strings.Contains(message.Text, label) {
			message.Text += "\n" + label + "."
		}
	}
	message.Images = images
	return message
}

// Compaction can remove an original while keeping a later reference (including
// the current user input). Move the needed pixels into that retained window
// before committing the new context, so the next request cannot contain a
// dangling reference. This runs only at an explicit context replacement.
func retainReferencedImages(before, after []model.Message) []model.Message {
	originals := make(map[string]model.Image)
	for _, message := range before {
		if !message.IsError {
			for _, picture := range message.Images {
				originals[imageHash(picture)] = picture
			}
		}
	}
	available := make(map[string]bool)
	for _, message := range after {
		if !message.IsError {
			for _, picture := range message.Images {
				available[imageHash(picture)] = true
			}
		}
	}
	after = slices.Clone(after)
	for i, message := range after {
		for _, key := range message.ImageRefs {
			if available[key] {
				continue
			}
			if picture, exists := originals[key]; exists {
				message.Images = append(slices.Clone(message.Images), picture)
				message.Text += "\nImage " + key[:16] + " restored here after context compaction."
				available[key] = true
			}
		}
		after[i] = message
	}
	return after
}
