package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"gcode/internal/model"
)

const toolResultInlineChars = 32_000

func resultHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func imageHash(image model.Image) string {
	return resultHash(fmt.Sprintf("%s:%d:%d:%s", image.MIME, image.Width, image.Height, resultHash(image.Data)))
}

// Decisions are made only for the new result. Searching the current context,
// rather than an ever-growing read cache, makes dedup safe after compact/resume.
func prepareToolResult(ctx context.Context, store func(context.Context, string) (string, error), history []model.Message, call model.Call, message model.Message) (model.Message, error) {
	if !message.IsError && call.Name == "Read" && len(message.Images) == 0 {
		key := resultHash(message.Text)
		for _, prior := range history {
			if prior.Role == "tool" && !prior.IsError && prior.ContentHash == key {
				message.Text = "Identical Read content is still available in tool result " + prior.CallID + "."
				if prior.OutputFile != "" {
					message.Text += "\nFull output: " + prior.OutputFile
				}
				return message, nil
			}
		}
		message.ContentHash = key
	}
	message.Images = slices.Clone(message.Images)
	for i := 0; i < len(message.Images); {
		key := imageHash(message.Images[i])
		previous := ""
		for _, prior := range history {
			if prior.Role != "tool" || prior.IsError {
				continue
			}
			for _, image := range prior.Images {
				if imageHash(image) == key {
					previous = prior.CallID
					break
				}
			}
			if previous != "" {
				break
			}
		}
		if previous == "" {
			i++
			continue
		}
		message.Text += "\nIdentical image is still available in tool result " + previous + "."
		message.Images = slices.Delete(message.Images, i, i+1)
	}
	if message.OutputFile == "" && (store == nil || utf8.RuneCountInString(message.Text) <= toolResultInlineChars) {
		return message, nil
	}
	path := message.OutputFile
	if path == "" {
		var err error
		path, err = store(ctx, message.Text)
		if err != nil {
			return message, fmt.Errorf("preserve full tool output: %w", err)
		}
	}
	if strings.TrimSpace(path) == "" {
		return message, errors.New("tool output storage returned an empty path")
	}
	runes := []rune(message.Text)
	message.OutputFile = path
	head := string(runes[:min(1536, len(runes))])
	tail := ""
	if len(runes) > 2048 {
		tail = "\n\n…\n\n" + string(runes[len(runes)-512:])
	}
	message.Text = fmt.Sprintf("Complete tool output saved to: %s\nUse Read with offset/limit, or a bounded file-processing command for very large files.\n\nPreview:\n%s%s", path, head, tail)
	return message, nil
}
