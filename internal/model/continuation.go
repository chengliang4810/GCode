package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
)

// Continuation retains adapter-owned replay data. Callers may persist or clone it,
// but only the model layer interprets it and checks the target before replay.
type Continuation struct {
	scope string
	data  json.RawMessage
}

func (c Continuation) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Scope string          `json:"scope"`
		Data  json.RawMessage `json:"data"`
	}{Scope: c.scope, Data: c.data})
}

func (c *Continuation) UnmarshalJSON(data []byte) error {
	var stored struct {
		Scope string          `json:"scope"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	c.scope, c.data = stored.Scope, bytes.Clone(stored.Data)
	return nil
}

func continuationScope(provider Provider, selected Model) string {
	// Keep credentials out of persisted data while separating account changes too.
	data, _ := json.Marshal([]string{
		provider.ID, string(provider.Protocol), provider.BaseURL, provider.APIKey, selected.ID,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func replayData(message Message, scope string) json.RawMessage {
	if message.Continuation == nil || message.Continuation.scope != scope {
		return nil
	}
	return message.Continuation.data
}

// CloneMessages gives workers independent ownership of execution history,
// including opaque adapter state.
func CloneMessages(history []Message) []Message {
	out := slices.Clone(history)
	for i := range out {
		out[i].InputIDs = slices.Clone(out[i].InputIDs)
		out[i].Calls = slices.Clone(out[i].Calls)
		out[i].Images = slices.Clone(out[i].Images)
		if c := out[i].Continuation; c != nil {
			out[i].Continuation = &Continuation{scope: c.scope, data: bytes.Clone(c.data)}
		}
	}
	return out
}
