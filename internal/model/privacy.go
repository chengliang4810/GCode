package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"gcode/internal/privacy"
	"gcode/third_party/magpie/pii"
	"gcode/third_party/magpie/redact"
)

// SetPrivacy freezes the settings and separates sealed replay from responses
// made with other masking choices. The caller owns the Gateway until Stream.
func (g *Gateway) SetPrivacy(settings privacy.Settings) {
	g.Privacy = settings.Clone()
	g.target.scope = privacyScope(continuationScope(g.provider, g.target.model), g.Privacy)
}

func (g *Gateway) maskRequest(ctx context.Context, body []byte) (out []byte, err error) {
	if !g.Privacy.Enabled {
		return body, nil
	}
	if err := g.Privacy.Validate(); err != nil {
		return nil, fmt.Errorf("invalid masking settings: %w", err)
	}
	// The imported recognizer may panic when a finder cannot run. Never send
	// the original body after a masking failure.
	defer func() {
		if recover() != nil {
			out, err = nil, errors.New("privacy masking failed; request was not sent")
			if ctx.Err() != nil {
				err = ctx.Err()
			}
		}
	}()
	report := privacy.Report{Kinds: map[string]int{}}
	options := g.Privacy.Options(g.target.scope)
	if g.Privacy.LocalModel && (g.Privacy.Secrets || g.Privacy.Personal) {
		if err := pii.PrepareCache(); err != nil {
			return nil, fmt.Errorf("local privacy model cache is unavailable: %w", err)
		}
		m := pii.Of(g.Privacy.ModelName)
		if !m.Installed() {
			return nil, errors.New("download the selected local privacy model before enabling it; request was not sent")
		}
		options.MinScore = pii.Threshold(g.Privacy.ModelScores, m)
		options.NamesByModel = m.ID() == pii.AstrLinkGuard
		options.Model = func(texts []string) [][]redact.Found {
			findCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			found, findErr := m.Find(findCtx, texts)
			if findErr != nil {
				panic(findErr) // caught above: never silently skip an enabled finder
			}
			for i := range found {
				found[i] = filterPrivateFinds(found[i], options.MinScore)
			}
			return found
		}
	}
	options.Seen = func(hit redact.Hit) {
		report.Kinds[hit.Kind]++
		switch hit.Action {
		case redact.Warn:
			report.Warned++
		case redact.Block:
			report.Blocked++
		default:
			report.Masked++
		}
	}
	out, _ = redact.MaskJSON(body, options)
	if g.PrivacyReport != nil && len(report.Kinds) > 0 {
		g.PrivacyReport(report)
	}
	if report.Blocked > 0 {
		return nil, errors.New("privacy rules blocked this model request; no request was sent")
	}
	if !json.Valid(out) {
		return nil, errors.New("privacy masking produced invalid JSON; request was not sent")
	}
	return out, nil
}

func filterPrivateFinds(found []redact.Found, threshold float64) []redact.Found {
	var out []redact.Found
	for _, value := range found {
		if value.Score >= threshold {
			out = append(out, value)
		}
	}
	return out
}

func (g *Gateway) decodePrivateStream(reader io.Reader, emit func(Delta) error) (Result, error) {
	if !g.Privacy.Enabled {
		return decodeStream(reader, g.adapter.decoder(), emit)
	}
	var text, reasoning redact.TextStream
	var deliveryErr error
	deliver := func(delta Delta) error {
		delta.Text, delta.Reasoning = text.Write(delta.Text), reasoning.Write(delta.Reasoning)
		if emit == nil {
			return nil
		}
		deliveryErr = emit(delta)
		return deliveryErr
	}
	result, err := decodeStream(reader, g.adapter.decoder(), deliver)
	// Flush partial literal text even on a failed attempt; retries reset the
	// UI stream in the existing retry path and start new restoration buffers.
	tail := Delta{Text: text.Finish(), Reasoning: reasoning.Finish()}
	if emit != nil && deliveryErr == nil && (tail.Text != "" || tail.Reasoning != "") {
		err = errors.Join(err, emit(tail))
	}
	result.Message.Text = redact.Restore(result.Message.Text, false)
	result.Message.Reasoning = redact.Restore(result.Message.Reasoning, false)
	for i := range result.Message.Calls {
		call := &result.Message.Calls[i]
		call.Arguments = string(redact.RestoreJSON([]byte(call.Arguments)))
		if !json.Valid([]byte(call.Arguments)) {
			result.Message.Calls = nil
			return result, errors.New("privacy restoration produced invalid tool arguments")
		}
	}
	// Continuation is adapter-owned provider text. Do not restore it: signed
	// thinking must be replayed byte-for-byte. Visible text and calls above
	// are the restored copies used by the Agent.
	return result, err
}
