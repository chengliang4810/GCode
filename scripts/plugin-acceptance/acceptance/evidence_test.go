package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func evidenceFixture(t *testing.T) (Report, RuntimeEvidence, string) {
	t.Helper()
	directory := t.TempDir()
	component := Component{ID: "skill:demo:run", Kind: "skill", Name: "demo:run", Digest: strings.Repeat("c", 64), Status: "not_tested"}
	row := Row{
		ID: "demo@fixture", MarketplaceCommit: strings.Repeat("a", 40), FixedSource: FixedSource{Commit: strings.Repeat("b", 40)},
		ArtifactDigest: strings.Repeat("d", 64), Platform: "linux/amd64", Model: "provider/model",
		Source: Stage{Status: "verified"}, Install: Stage{Status: "passed"}, Discover: Stage{Status: "passed"}, Runtime: Stage{Status: "not_tested"}, Components: []Component{component},
	}
	start := time.Now().UTC().Add(-time.Second)
	probe := Probe{ID: "probe-1", ComponentID: component.ID, ComponentDigest: component.Digest, Operation: "skill-invoke", Outcome: "passed", StartedAt: start, FinishedAt: start.Add(time.Second)}
	evidence := RuntimeEvidence{SchemaVersion: 1, RunID: "run-1", PluginID: row.ID, MarketplaceCommit: row.MarketplaceCommit, SourceCommit: row.FixedSource.Commit, ArtifactDigest: row.ArtifactDigest, Platform: row.Platform, Model: row.Model, Probes: []Probe{probe}}
	writeTrace(t, &evidence, row, directory)
	return Report{Rows: []Row{row}, Denominator: 1, RequiredPasses: 1, Directory: directory}, evidence, directory
}

func writeTrace(t *testing.T, evidence *RuntimeEvidence, row Row, directory string) {
	t.Helper()
	for i := range evidence.Probes {
		probe := &evidence.Probes[i]
		var log []byte
		for _, phase := range []string{"invoke", "result"} {
			payload := json.RawMessage(`{"prompt":"Run this skill against the isolated fixture"}`)
			at := probe.StartedAt
			if phase == "result" {
				payload = json.RawMessage(`{"response":"Verified fixture output","providerRequestId":"request-actual-log-id"}`)
				at = probe.FinishedAt
			}
			record := TraceRecord{RunID: evidence.RunID, PluginID: evidence.PluginID, MarketplaceCommit: evidence.MarketplaceCommit, SourceCommit: evidence.SourceCommit, ArtifactDigest: evidence.ArtifactDigest,
				Platform: evidence.Platform, Model: evidence.Model, ProbeID: probe.ID, ComponentID: probe.ComponentID, ComponentDigest: probe.ComponentDigest,
				Operation: probe.Operation, Phase: phase, At: at, Payload: payload}
			data, err := json.Marshal(record)
			if err != nil {
				t.Fatal(err)
			}
			log = append(log, data...)
			log = append(log, '\n')
		}
		path := filepath.Join(directory, probe.ID+".jsonl")
		if err := os.WriteFile(path, log, 0600); err != nil {
			t.Fatal(err)
		}
		probe.Log = EvidenceFile{Path: path, SHA256: hash(log)}
	}
}

func importFixture(t *testing.T, report *Report, evidence RuntimeEvidence, directory string) error {
	t.Helper()
	path := filepath.Join(directory, "evidence.json")
	if err := writeJSON(path, evidence); err != nil {
		t.Fatal(err)
	}
	return ImportEvidence(report, path)
}

func TestEvidenceRequiresCompleteBoundInvocationAndResult(t *testing.T) {
	t.Parallel()
	report, evidence, directory := evidenceFixture(t)
	if err := importFixture(t, &report, evidence, directory); err != nil {
		t.Fatal(err)
	}
	if report.Passed != 1 || !report.TargetMet {
		t.Fatal("complete fixture trace did not pass the validation contract")
	}
	// This is a deterministic evidence-validator fixture, not a real provider claim.
	if err := importFixture(t, &report, evidence, directory); err == nil {
		t.Fatal("same runtime result counted twice")
	}
	if report.Passed != 1 || report.Denominator != 1 {
		t.Fatal("duplicate evidence changed denominator/numerator")
	}
}

func TestEvidenceRejectsWrongIdentityOrPartialComponents(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"version", "platform", "model", "artifact", "component-digest", "missing-component", "duplicate-probe", "boolean-only", "log-tamper"} {
		t.Run(name, func(t *testing.T) {
			report, evidence, directory := evidenceFixture(t)
			switch name {
			case "version":
				evidence.SourceCommit = strings.Repeat("e", 40)
			case "platform":
				evidence.Platform = "windows/amd64"
			case "model":
				evidence.Model = "provider/other-model"
			case "artifact":
				evidence.ArtifactDigest = strings.Repeat("f", 64)
			case "component-digest":
				evidence.Probes[0].ComponentDigest = strings.Repeat("f", 64)
			case "missing-component":
				report.Rows[0].Components = append(report.Rows[0].Components, Component{ID: "mcp:another", Kind: "mcp", Digest: "another", Status: "not_tested"})
			case "duplicate-probe":
				evidence.Probes = append(evidence.Probes, evidence.Probes[0])
			case "boolean-only":
				data, err := os.ReadFile(evidence.Probes[0].Log.Path)
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.ReplaceAll(string(data), `{"response":"Verified fixture output","providerRequestId":"request-actual-log-id"}`, `{"passed":true}`))
				if err := os.WriteFile(evidence.Probes[0].Log.Path, data, 0600); err != nil {
					t.Fatal(err)
				}
				evidence.Probes[0].Log.SHA256 = hash(data)
			case "log-tamper":
				if err := os.WriteFile(evidence.Probes[0].Log.Path, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := importFixture(t, &report, evidence, directory); err == nil {
				t.Fatal("invalid runtime evidence accepted")
			}
			recount(&report)
			if report.Passed != 0 || report.TargetMet {
				t.Fatal("invalid evidence increased acceptance")
			}
		})
	}
}

func TestNeedsAuthAndUnavailableStayInDenominator(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"needs_auth", "unavailable", "blocked", "failed"} {
		t.Run(outcome, func(t *testing.T) {
			report, evidence, directory := evidenceFixture(t)
			evidence.Probes[0].Outcome = outcome
			evidence.Probes[0].Error = "Explicit external test credentials are missing"
			evidence.Probes[0].Log = EvidenceFile{}
			if err := importFixture(t, &report, evidence, directory); err != nil {
				t.Fatal(err)
			}
			if report.Denominator != 1 || report.Passed != 0 || report.TargetMet {
				t.Fatal("runtime requirement was removed from denominator")
			}
		})
	}
}

func TestDiscoveryOrSuccessBooleansCannotReplaceRuntime(t *testing.T) {
	t.Parallel()
	report, evidence, directory := evidenceFixture(t)
	report.Rows[0].Source.Status = "snapshot_only"
	if err := importFixture(t, &report, evidence, directory); err == nil {
		t.Fatal("static source snapshot was promoted by imported runtime evidence")
	}
	if substantivePayload(json.RawMessage(`{"passed":true,"status":"passed","success":true}`)) {
		t.Fatal("status booleans considered actual operation output")
	}
}
