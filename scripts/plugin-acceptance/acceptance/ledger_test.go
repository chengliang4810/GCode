package acceptance

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOfficialFrozenDenominatorIncludesEveryUntestedPlugin(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "..", DefaultMarketplaceFile)
	report, err := Execute(context.Background(), Config{MarketplaceFile: path, RequireOfficialBaseline: true, OutputDir: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Denominator != 315 || report.RequiredPasses != 284 || report.Passed != 0 || report.TargetMet {
		t.Fatalf("incorrect official acceptance arithmetic: %+v", report)
	}
	seen := map[string]bool{}
	for _, row := range report.Rows {
		if seen[row.ID] {
			t.Fatal("duplicate denominator ID")
		}
		seen[row.ID] = true
		if row.Runtime.Status != "not_tested" || row.Passed {
			t.Fatalf("static snapshot became runtime success: %+v", row)
		}
		if row.Install.Status != "not_tested" || row.Discover.Status != "not_tested" {
			t.Fatal("snapshot was misrepresented as an install/discovery run")
		}
	}
	file, err := os.Open(filepath.Join(report.Directory, "ledger.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 316 {
		t.Fatalf("CSV lost untested denominator rows: %d", len(records))
	}
}

func TestModifiedOfficialSnapshotCannotClaimFrozenBaseline(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "market.json")
	if err := os.WriteFile(path, []byte(`{"name":"claude-plugins-official","plugins":[{"name":"only","source":"./only"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(context.Background(), Config{MarketplaceFile: path, RequireOfficialBaseline: true, OutputDir: t.TempDir()}, nil); err == nil {
		t.Fatal("modified official denominator accepted")
	}
}

func TestOfficialDiscoveryWithoutCacheRecordsAllBlockedSources(t *testing.T) {
	t.Parallel()
	report, err := Execute(context.Background(), Config{MarketplaceFile: filepath.Join("..", "..", "..", DefaultMarketplaceFile), RequireOfficialBaseline: true, OutputDir: t.TempDir(), InstallAndDiscover: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Denominator != 315 || len(report.Rows) != 315 || report.Passed != 0 {
		t.Fatal("official offline discovery changed acceptance scope")
	}
	for _, row := range report.Rows {
		if row.Source.Status != "blocked" || row.Install.Status != "not_tested" {
			t.Fatal("cache miss was not retained as a blocked source")
		}
	}
}

func TestCustomDenominatorNeverExcludesUnavailableOrBlockedRows(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "market.json")
	if err := os.WriteFile(path, []byte(`{"name":"fixture","plugins":[{"name":"same","source":"./same"},{"name":"auth","source":"./auth"},{"name":"missing","source":"./missing"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Execute(context.Background(), Config{MarketplaceFile: path, OutputDir: t.TempDir(), InstallAndDiscover: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Denominator != 3 || report.RequiredPasses != 3 || report.Passed != 0 || len(report.Rows) != 3 {
		t.Fatal("blocked sources changed denominator")
	}
	for _, row := range report.Rows {
		if row.Source.Status != "blocked" || row.Passed {
			t.Fatal("missing cache was counted as accepted")
		}
	}
}

func TestDuplicateNamesCannotCountAsDifferentVersions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "market.json")
	if err := os.WriteFile(path, []byte(`{"name":"fixture","plugins":[{"name":"same","version":"1","source":"./one"},{"name":"same","version":"2","source":"./two"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Execute(context.Background(), Config{MarketplaceFile: path, OutputDir: t.TempDir()}, nil); err == nil {
		t.Fatal("duplicate name/version rows inflated denominator")
	}
}

func TestRunRequiresExplicitPrivateProviderAndModel(t *testing.T) {
	t.Parallel()
	for _, cfg := range []Config{{Run: true}, {Run: true, ProviderConfig: "explicit.json"}} {
		if _, err := Execute(context.Background(), cfg, nil); err == nil {
			t.Fatal("runtime accepted without explicit private provider and model")
		}
	}
}

func TestReportRetainsFailedImportEvidence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "market.json")
	if err := os.WriteFile(path, []byte(`{"name":"fixture","plugins":[{"name":"only","source":"./only"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(evidence, []byte(`{"schemaVersion":1,"pluginId":"only@fixture","passed":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Execute(context.Background(), Config{MarketplaceFile: path, OutputDir: t.TempDir(), Model: "test/model", RuntimeEvidence: []string{evidence}}, nil)
	if err == nil {
		t.Fatal("unbound success boolean accepted")
	}
	data, readErr := os.ReadFile(filepath.Join(report.Directory, "ledger.json"))
	if readErr != nil {
		t.Fatal("failed run lost its ledger:", readErr)
	}
	var saved Report
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Passed != 0 || saved.Denominator != 1 {
		t.Fatal("failure ledger misreported acceptance")
	}
}
