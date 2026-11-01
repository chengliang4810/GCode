// Command plugin-acceptance creates a frozen, per-plugin acceptance ledger.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"time"

	"gcode/scripts/plugin-acceptance/acceptance"
	"gcode/scripts/plugin-acceptance/corerunner"
)

type stringFlags []string

func (s *stringFlags) String() string         { return fmt.Sprint([]string(*s)) }
func (s *stringFlags) Set(value string) error { *s = append(*s, value); return nil }

func main() {
	var cfg acceptance.Config
	var evidence stringFlags
	flag.StringVar(&cfg.MarketplaceFile, "marketplace-file", acceptance.DefaultMarketplaceFile, "Frozen marketplace JSON; default is the official 315-entry snapshot")
	flag.StringVar(&cfg.MarketplaceCommit, "marketplace-commit", acceptance.OfficialCommit, "Full frozen marketplace commit")
	flag.StringVar(&cfg.MarketplaceRepository, "marketplace-repository", acceptance.OfficialRepository, "Marketplace repository identity")
	flag.StringVar(&cfg.SourceEvidenceDir, "snapshot-dir", "", "Previous static source evidence directory (never counts as runtime success)")
	flag.StringVar(&cfg.SourceRoot, "source-root", "", "Local Git repository containing the frozen marketplace source commit")
	flag.StringVar(&cfg.Cache, "cache", "", "Offline cache index JSON or directory containing index.json")
	flag.StringVar(&cfg.OutputDir, "out-dir", "output/plugin-acceptance", "Parent for a fresh per-run private evidence directory")
	flag.StringVar(&cfg.Platform, "platform", runtime.GOOS+"/"+runtime.GOARCH, "Exact operating system/architecture acceptance identity")
	flag.StringVar(&cfg.Model, "model", "", "Exact provider/model identity for runtime evidence")
	flag.IntVar(&cfg.Workers, "workers", 4, "Concurrent source workers, 1 through 16; actual model probes run sequentially")
	flag.DurationVar(&cfg.Timeout, "timeout", 2*time.Minute, "Per-plugin source/install/runtime deadline")
	flag.BoolVar(&cfg.InstallAndDiscover, "install-and-discover", false, "Verify fixed local Git sources, then use the native plugin manager and bundle loader; does not execute plugin code")
	flag.BoolVar(&cfg.Run, "run", false, "Run actual component probes through a linked RuntimeRunner; requires --provider-config")
	flag.StringVar(&cfg.ProviderConfig, "provider-config", "", "Explicit private test provider JSON; never reads the user's .gcode profile")
	flag.StringVar(&cfg.ProbeDir, "probe-dir", "", "Explicit per-plugin business probe plan directory")
	flag.Var(&evidence, "runtime-evidence", "Import a component runtime evidence JSON file; may be repeated")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "plugin-acceptance accepts flags only")
		os.Exit(2)
	}
	cfg.RuntimeEvidence = []string(evidence)
	cfg.RequireOfficialBaseline = cfg.MarketplaceFile == acceptance.DefaultMarketplaceFile
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	report, err := acceptance.Execute(ctx, cfg, corerunner.InProcessRunner{})
	if report.Directory != "" {
		fmt.Printf("Evidence: %s\nRuntime acceptance: %d/%d (%.2f%%), required %d; target met: %t\n", report.Directory, report.Passed, report.Denominator, report.Percent, report.RequiredPasses, report.TargetMet)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "plugin-acceptance:", err)
		os.Exit(1)
	}
	if cfg.Run && !report.TargetMet {
		fmt.Fprintln(os.Stderr, "plugin-acceptance: runtime acceptance target has not been met; missing plans, unsupported components and unmet assertions remain in the denominator")
		os.Exit(1)
	}
}
