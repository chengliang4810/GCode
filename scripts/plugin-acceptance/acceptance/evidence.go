package acceptance

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ImportEvidence accepts one plugin execution record. Version/platform/model rows
// remain distinct; the denominator never shrinks when evidence is unavailable.
func ImportEvidence(report *Report, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) > 16<<20 {
		return errors.New("runtime evidence exceeds 16 MiB")
	}
	var evidence RuntimeEvidence
	if err := strictJSON(data, &evidence); err != nil {
		return fmt.Errorf("runtime evidence: %w", err)
	}
	for i := range report.Rows {
		row := &report.Rows[i]
		if row.ID != evidence.PluginID {
			continue
		}
		if row.Runtime.Status != "not_tested" {
			return fmt.Errorf("duplicate runtime evidence for %s/%s/%s", row.ID, row.Platform, row.Model)
		}
		if err := applyEvidence(row, evidence, filepath.Dir(path)); err != nil {
			row.Runtime = Stage{Status: "rejected", Error: err.Error()}
			row.Errors = append(row.Errors, err.Error())
			return err
		}
		row.Evidence = append(row.Evidence, path)
		recount(report)
		return nil
	}
	return fmt.Errorf("evidence plugin %q is not in the frozen marketplace denominator", evidence.PluginID)
}

func applyEvidence(row *Row, evidence RuntimeEvidence, base string) error {
	if evidence.SchemaVersion != 1 || evidence.RunID == "" {
		return errors.New("runtime evidence requires schemaVersion=1 and runId")
	}
	if row.Source.Status != "verified" || row.Install.Status != "passed" || row.Discover.Status != "passed" || row.ArtifactDigest == "" {
		return errors.New("runtime evidence cannot replace verified source/install/discover stages")
	}
	if len(row.Blocked) != 0 {
		return errors.New("plugin has unresolved component or source blockers")
	}
	if evidence.PluginID != row.ID || evidence.MarketplaceCommit != row.MarketplaceCommit || evidence.SourceCommit != row.FixedSource.Commit ||
		evidence.ArtifactDigest != row.ArtifactDigest || evidence.Platform != row.Platform || evidence.Model != row.Model || evidence.Model == "" {
		return errors.New("runtime evidence identity/version/platform/model/artifact does not match the installed frozen row")
	}
	if len(evidence.Probes) == 0 {
		return errors.New("runtime evidence has no actual component probes")
	}
	components := map[string]Component{}
	for _, component := range row.Components {
		if _, duplicate := components[component.ID]; duplicate {
			return errors.New("ambiguous duplicate discovered component ID")
		}
		components[component.ID] = component
	}
	seenProbes, seenComponents := map[string]bool{}, map[string]bool{}
	status := "passed"
	validated := make([]Probe, 0, len(evidence.Probes))
	for _, probe := range evidence.Probes {
		component, exists := components[probe.ComponentID]
		if !exists || probe.ComponentDigest != component.Digest {
			return fmt.Errorf("probe %s is not bound to a discovered component digest", probe.ID)
		}
		if probe.ID == "" || seenProbes[probe.ID] || seenComponents[probe.ComponentID] {
			return errors.New("duplicate probe or component result")
		}
		seenProbes[probe.ID], seenComponents[probe.ComponentID] = true, true
		if probe.StartedAt.IsZero() || probe.FinishedAt.Before(probe.StartedAt) {
			return errors.New("probe timestamps are missing or inverted")
		}
		if !operationAllowed(component.Kind, probe.Operation) {
			return fmt.Errorf("operation %q is not an execution probe for component %s", probe.Operation, component.ID)
		}
		switch probe.Outcome {
		case "passed":
			if err := validateTrace(row, evidence, component, probe, base); err != nil {
				return err
			}
		case "failed", "blocked", "needs_auth", "unavailable":
			if probe.Error == "" {
				return errors.New("nonpassing probe must explain its missing runtime requirement")
			}
			status = probe.Outcome
			if probe.Outcome == "unavailable" {
				row.Unavailable = true
			}
		default:
			return errors.New("unknown runtime probe outcome")
		}
		validated = append(validated, probe)
	}
	for _, component := range row.Components {
		if !seenComponents[component.ID] {
			return fmt.Errorf("declared component %s has no verified runtime result", component.ID)
		}
	}
	if len(row.Components) == 0 {
		return errors.New("empty plugin cannot obtain runtime acceptance")
	}
	for i := range row.Components {
		for _, probe := range validated {
			if probe.ComponentID == row.Components[i].ID {
				row.Components[i].Status = probe.Outcome
				break
			}
		}
	}
	row.Probes = validated
	row.Runtime = Stage{Status: status, Method: "bound-component-invoke-result-traces"}
	return nil
}

func operationAllowed(kind, operation string) bool {
	expected := map[string]string{
		"skill": "skill-invoke", "command": "command-invoke", "agent": "agent-run", "hook": "hook-event", "mcp": "mcp-tool-call", "lsp": "lsp-request",
		"output-style": "output-style-apply", "settings": "settings-apply", "dependency": "dependency-capability", "user-config": "user-config-resolve", "channel": "channel-message",
	}
	return expected[kind] != "" && expected[kind] == operation
}

func validateTrace(row *Row, evidence RuntimeEvidence, component Component, probe Probe, base string) error {
	if probe.Log.Path == "" || len(probe.Log.SHA256) != 64 {
		return errors.New("passing probe requires a log path and SHA-256")
	}
	logPath := probe.Log.Path
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(base, logPath)
	}
	info, err := os.Lstat(logPath)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return errors.New("probe log is not a bounded regular file")
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		return err
	}
	if hash(data) != probe.Log.SHA256 {
		return fmt.Errorf("probe %s log digest mismatch", probe.ID)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64<<10), 2<<20)
	invokes, results := 0, 0
	var invoked, finished TraceRecord
	for scanner.Scan() {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var record TraceRecord
		if err := strictJSON(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("probe trace: %w", err)
		}
		if record.RunID != evidence.RunID || record.PluginID != row.ID || record.MarketplaceCommit != row.MarketplaceCommit || record.SourceCommit != row.FixedSource.Commit ||
			record.ArtifactDigest != row.ArtifactDigest || record.Platform != row.Platform || record.Model != row.Model || record.ProbeID != probe.ID ||
			record.ComponentID != component.ID || record.ComponentDigest != component.Digest || record.Operation != probe.Operation {
			return fmt.Errorf("probe %s trace is not bound to its exact plugin/component/source/platform/model", probe.ID)
		}
		if record.At.Before(probe.StartedAt) || record.At.After(probe.FinishedAt) || record.Error != "" {
			return errors.New("passing trace contains an error or timestamp outside the probe")
		}
		switch record.Phase {
		case "invoke":
			invokes++
			invoked = record
		case "result":
			results++
			finished = record
		default:
			return errors.New("runtime probe trace requires invoke/result events")
		}
		if !substantivePayload(record.Payload) {
			return errors.New("runtime trace needs actual structured input/output; a status/passed boolean is insufficient")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if invokes != 1 || results != 1 || finished.At.Before(invoked.At) {
		return errors.New("passing probe needs one invocation and one subsequent result")
	}
	return validateOperationPayload(component.Kind, invoked.Payload, finished.Payload)
}

func substantivePayload(raw json.RawMessage) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch value := value.(type) {
	case map[string]any:
		for key, v := range value {
			if key == "passed" || key == "success" || key == "status" || key == "ok" {
				continue
			}
			switch item := v.(type) {
			case string:
				if strings.TrimSpace(item) != "" {
					return true
				}
			case map[string]any:
				if len(item) > 0 {
					return true
				}
			case []any:
				if len(item) > 0 {
					return true
				}
			case float64:
				if key != "exitCode" {
					return true
				}
			}
		}
	}
	return false
}

func validateOperationPayload(kind string, input, output json.RawMessage) error {
	var in, out map[string]json.RawMessage
	_ = json.Unmarshal(input, &in)
	_ = json.Unmarshal(output, &out)
	require := func(fields map[string]json.RawMessage, keys ...string) bool {
		for _, key := range keys {
			if raw, ok := fields[key]; ok && len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
				return true
			}
		}
		return false
	}
	valid := false
	switch kind {
	case "skill", "command", "agent", "output-style", "settings":
		valid = require(in, "prompt", "messages") && require(out, "response", "messages", "artifact") && require(out, "providerRequestId", "modelResponseId")
	case "hook":
		valid = require(in, "hook_event_name") && require(out, "stdout", "response", "result") && require(out, "exitCode", "transport")
	case "mcp":
		var method string
		_ = json.Unmarshal(in["method"], &method)
		valid = method == "tools/call" && require(in, "params") && require(out, "content", "result") && !require(out, "error")
	case "lsp":
		var method string
		_ = json.Unmarshal(in["method"], &method)
		valid = strings.HasPrefix(method, "textDocument/") && require(in, "params") && require(out, "result") && !require(out, "error")
	case "dependency":
		valid = require(in, "dependencyId") && require(out, "capabilityResult", "response")
	case "user-config":
		valid = require(in, "option") && require(out, "resolvedDigest", "redactedValue")
	case "channel":
		valid = require(in, "notification") && require(out, "receivedMessage", "response")
	}
	if !valid {
		return fmt.Errorf("runtime probe payload lacks the actual %s operation/result fields", kind)
	}
	return nil
}

func strictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("evidence must contain exactly one JSON object")
	}
	return nil
}

func WriteReport(report Report) error {
	if report.Directory == "" {
		return errors.New("report directory is missing")
	}
	if err := writeJSON(filepath.Join(report.Directory, "ledger.json"), report); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(report.Directory, "ledger.csv"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	header := []string{"id", "marketplace_commit", "source_repository", "source_commit", "source_path", "platform", "model", "version", "source", "install", "discover", "runtime", "passed", "unavailable", "blocked", "components", "probes", "artifact_digest", "evidence", "errors"}
	if err := writer.Write(header); err != nil {
		_ = file.Close()
		return err
	}
	encoded := func(value any) string { data, _ := json.Marshal(value); return string(data) }
	for _, row := range report.Rows {
		values := []string{row.ID, row.MarketplaceCommit, row.FixedSource.Repository, row.FixedSource.Commit, row.FixedSource.Path, row.Platform, row.Model, row.Version,
			row.Source.Status, row.Install.Status, row.Discover.Status, row.Runtime.Status, strconv.FormatBool(row.Passed), strconv.FormatBool(row.Unavailable),
			encoded(row.Blocked), encoded(row.Components), encoded(row.Probes), row.ArtifactDigest, encoded(row.Evidence), encoded(row.Errors)}
		if err := writer.Write(values); err != nil {
			_ = file.Close()
			return err
		}
	}
	writer.Flush()
	return errors.Join(writer.Error(), file.Close())
}
