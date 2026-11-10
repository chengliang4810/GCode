package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// RequestDiagnostics fingerprints the encoded wire request, including opaque
// continuation and image bytes, without storing those bytes a second time.
// Message hashes ignore moving cache_control annotations; SHA256 covers the
// exact body, including those annotations.
type RequestDiagnostics struct {
	Bytes         int      `json:"bytes"`
	SHA256        string   `json:"sha256"`
	SystemHash    string   `json:"systemHash"`
	ToolsHash     string   `json:"toolsHash"`
	ParameterHash string   `json:"parameterHash"`
	TargetHash    string   `json:"targetHash"`
	MessageHashes []string `json:"messageHashes"`
	MessageCount  int      `json:"messageCount"`
	ImageBytes    int      `json:"imageBytes"`
	ImageHashes   []string `json:"imageHashes,omitempty"`
}

func diagnosticHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func diagnosticValueHash(value any) string {
	data, _ := json.Marshal(value)
	return diagnosticHash(data)
}

func requestDiagnostics(payload map[string]any, body []byte, request Request) RequestDiagnostics {
	result := RequestDiagnostics{Bytes: len(body), SHA256: diagnosticHash(body), ToolsHash: diagnosticValueHash(payload["tools"])}
	system := payload["system"]
	if system == nil {
		system = payload["instructions"]
	}
	if system == nil {
		if messages, ok := payload["messages"].([]any); ok && len(messages) > 0 {
			if first, ok := messages[0].(map[string]any); ok && first["role"] == "system" {
				system = first["content"]
			}
		}
	}
	result.SystemHash = diagnosticValueHash(system)
	parameters := make(map[string]any)
	for key, value := range payload {
		switch key {
		case "messages", "input":
			messages, _ := value.([]any)
			result.MessageCount += len(messages)
			for _, message := range messages {
				if len(result.MessageHashes) < 4096 {
					result.MessageHashes = append(result.MessageHashes, diagnosticValueHash(withoutCacheMarkers(message)))
				}
			}
		case "system", "instructions", "tools":
		default:
			parameters[key] = value
		}
	}
	result.ParameterHash = diagnosticValueHash(parameters)
	for _, message := range request.History {
		for _, image := range message.Images {
			result.ImageBytes += len(image.Data)
			result.ImageHashes = append(result.ImageHashes, diagnosticValueHash(image))
		}
	}
	return result
}

func withoutCacheMarkers(value any) any {
	message, ok := diagnosticBlock(value)
	if !ok {
		return value
	}
	content, ok := message["content"].([]any)
	if !ok {
		return value
	}
	copy := make(map[string]any, len(message))
	for key, item := range message {
		copy[key] = item
	}
	blocks := make([]any, len(content))
	for i, item := range content {
		block, ok := diagnosticBlock(item)
		if !ok {
			blocks[i] = item
			continue
		}
		plain := make(map[string]any, len(block))
		for key, field := range block {
			if key != "cache_control" {
				plain[key] = field
			}
		}
		blocks[i] = plain
	}
	copy["content"] = blocks
	return copy
}

func diagnosticBlock(value any) (map[string]any, bool) {
	if raw, ok := value.(json.RawMessage); ok {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var block map[string]any
		err := decoder.Decode(&block)
		return block, err == nil
	}
	block, ok := value.(map[string]any)
	return block, ok
}

type TransportDiagnostics struct {
	UploadCompleteMS  *int64 `json:"uploadCompleteMs,omitempty"`
	ResponseHeadersMS *int64 `json:"responseHeadersMs,omitempty"`
	RequestID         string `json:"requestId,omitempty"`
}

// CacheChange explains observed client changes; it never asserts what a
// provider cached, evicted, executed or billed.
type CacheChange struct {
	ComparedTo           string   `json:"comparedTo,omitempty"`
	Changes              []string `json:"changes,omitempty"`
	CommonPrefixMessages int      `json:"commonPrefixMessages"`
	Expected             bool     `json:"expected"`
	ReadDrop             bool     `json:"readDrop"`
}
