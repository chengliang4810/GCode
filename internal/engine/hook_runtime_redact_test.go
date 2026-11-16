package engine

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"gcode/internal/agent"
	"gcode/internal/contracts"
)

func TestHookRedactsSearchTokenFromAllDiagnosticForms(t *testing.T) {
	token := "search-secret +/\"\\&中文"
	core := &Engine{state: contracts.Snapshot{Settings: contracts.Settings{
		General: contracts.GeneralSettings{WebSearch: contracts.WebSearchSettings{Token: token}},
	}}}
	encoded, _ := json.Marshal(token)
	text := strings.Join([]string{token, url.QueryEscape(token), url.PathEscape(token), string(encoded[1 : len(encoded)-1])}, " | ")
	redact := core.hookTextRedactor(contracts.HookRule{})
	result := redactHookResultWith(agent.HookResult{
		Context: text, Reason: text, SystemMessage: text, StopReason: text,
		InitialUserMessage: text, SessionTitle: text, DisplayContent: &text,
	}, redact)
	data, _ := json.Marshal(result)
	for _, value := range []string{token, url.QueryEscape(token), url.PathEscape(token)} {
		if strings.Contains(string(data), value) {
			t.Fatalf("search credential remained in hook result")
		}
	}
	if strings.Contains(redact(text), string(encoded[1:len(encoded)-1])) {
		t.Fatal("JSON-escaped credential remained in diagnostics")
	}
	if redact("ordinary output") != "ordinary output" {
		t.Fatal("redaction changed unrelated output")
	}
}
