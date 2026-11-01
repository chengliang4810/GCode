// Command desktop-e2e builds and tests the native desktop on the current OS.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"gcode/internal/processutil"
	"gcode/scripts/frameworkpin"
)

func main() {
	out := flag.String("out-dir", "output/desktop-e2e", "Evidence directory within this checkout")
	filter := flag.String("run", "TestDesktop", "Go test name expression")
	timeout := flag.Duration("timeout", 4*time.Minute, "Test suite timeout")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "desktop-e2e accepts flags only")
		os.Exit(2)
	}
	if err := execute(*out, *filter, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "desktop-e2e:", err)
		os.Exit(1)
	}
}

type evidence struct {
	SchemaVersion int      `json:"schemaVersion"`
	Status        string   `json:"status"`
	GOOS          string   `json:"goos"`
	GOARCH        string   `json:"goarch"`
	Renderer      string   `json:"renderer"`
	Automation    string   `json:"automation"`
	Tests         string   `json:"tests"`
	Binary        string   `json:"binary"`
	BinarySHA256  string   `json:"binarySha256,omitempty"`
	PassedTests   []string `json:"passedTests"`
	BuildTags     []string `json:"buildTags"`
	TestExitCode  *int     `json:"testExitCode"`
	VisualParity  string   `json:"visualParity"`
	RealProvider  string   `json:"realProvider"`
}

func execute(out, filter string, timeout time.Duration) (resultErr error) {
	if _, err := regexp.Compile(filter); err != nil {
		return fmt.Errorf("invalid -run expression: %w", err)
	}
	if timeout <= 0 {
		return errors.New("-timeout must be positive")
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return errors.New("native desktop tests require a graphical session; on Linux CI run under Xvfb")
	}
	root, err := checkoutRoot()
	if err != nil {
		return err
	}
	output, err := evidenceDirectory(root, out)
	if err != nil {
		return err
	}
	runDir, err := os.MkdirTemp(output, time.Now().Format("20060102-150405")+"-")
	if err != nil {
		return err
	}
	binary := filepath.Join(runDir, binaryName(runtime.GOOS))
	report := evidence{
		SchemaVersion: 1, Status: "verification-failed", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Renderer: "MyGo native window", Automation: "github.com/egoist/mygo/plugins/automation",
		Tests: filter, Binary: binary, PassedTests: []string{}, BuildTags: []string{"e2e"},
		VisualParity: "not-assessed", RealProvider: "not-tested",
	}
	fmt.Println("Evidence:", runDir)
	defer func() {
		if digest, err := fileHash(binary); err == nil {
			report.BinarySHA256 = digest
		}
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			err = os.WriteFile(filepath.Join(runDir, "evidence.json"), append(data, '\n'), 0600)
		}
		resultErr = errors.Join(resultErr, err)
	}()

	lock, err := frameworkpin.VerifySource(root)
	if err != nil {
		return err
	}
	deps := exec.Command("go", "list", "-deps", "./cmd/rcode")
	processutil.HideConsole(deps)
	deps.Dir = root
	dependencies, err := deps.Output()
	if err != nil {
		return fmt.Errorf("normal desktop dependency check: %w", err)
	}
	if slices.Contains(strings.Fields(string(dependencies)), report.Automation) {
		return errors.New("automation must be excluded from normal desktop builds")
	}
	report.Status = "build-failed"
	buildLog, err := os.Create(filepath.Join(runDir, "build.log"))
	if err != nil {
		return err
	}
	build := exec.Command("go", "build", "-tags", "e2e", "-ldflags", linkerFlags(runtime.GOOS, frameworkpin.LinkerFlags(lock)), "-o", binary, "./cmd/rcode")
	processutil.HideConsole(build)
	build.Dir = root
	build.Stdout, build.Stderr = io.MultiWriter(os.Stdout, buildLog), io.MultiWriter(os.Stderr, buildLog)
	buildErr := build.Run()
	closeErr := buildLog.Close()
	if err := errors.Join(buildErr, closeErr); err != nil {
		return fmt.Errorf("build native desktop: %w", err)
	}
	report.Status = "test-failed"
	passed, exit, err := runTests(root, runDir, binary, filter, timeout)
	report.PassedTests, report.TestExitCode = passed, exit
	if err != nil {
		return err
	}
	if len(passed) == 0 {
		return errors.New("no desktop tests passed; check the -run filter (skipped tests do not count)")
	}
	report.Status = "passed"
	fmt.Printf("Desktop E2E passed (%d tests). Evidence: %s\n", len(passed), runDir)
	return nil
}

func runTests(root, runDir, binary, filter string, timeout time.Duration) ([]string, *int, error) {
	textLog, err := os.Create(filepath.Join(runDir, "tests.log"))
	if err != nil {
		return nil, nil, err
	}
	defer textLog.Close()
	jsonLog, err := os.Create(filepath.Join(runDir, "tests.jsonl"))
	if err != nil {
		return nil, nil, err
	}
	defer jsonLog.Close()
	test := exec.Command("go", "test", "-json", "-tags", "e2e", "./internal/e2e", "-run", filter, "-count=1", "-timeout", timeout.String())
	processutil.HideConsole(test)
	test.Dir = root
	test.Env = testEnvironment(os.Environ(), binary, runDir)
	test.Stderr = io.MultiWriter(os.Stderr, textLog)
	stdout, err := test.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := test.Start(); err != nil {
		return nil, nil, err
	}
	stream := io.TeeReader(stdout, jsonLog)
	passed, decodeErr := collectResults(stream, io.MultiWriter(os.Stdout, textLog))
	if decodeErr != nil {
		// Drain before Wait so malformed output cannot block the child on stdout.
		_, _ = io.Copy(io.Discard, stream)
	}
	waitErr := test.Wait()
	exit := test.ProcessState.ExitCode()
	if err := errors.Join(waitErr, decodeErr); err != nil {
		return passed, &exit, fmt.Errorf("native tests failed: %w", err)
	}
	return passed, &exit, nil
}

func collectResults(reader io.Reader, output io.Writer) ([]string, error) {
	passed := []string{}
	decoder := json.NewDecoder(reader)
	for {
		var event struct{ Action, Test, Output string }
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) {
				slices.Sort(passed)
				return slices.Compact(passed), nil
			}
			return passed, fmt.Errorf("decode go test events: %w", err)
		}
		if _, err := io.WriteString(output, event.Output); err != nil {
			return passed, err
		}
		if event.Action == "pass" && strings.HasPrefix(event.Test, "TestDesktop") && !strings.Contains(event.Test, "/") {
			passed = append(passed, event.Test)
		}
	}
}

func binaryName(goos string) string {
	if goos == "windows" {
		return "rcode-e2e.exe"
	}
	return "rcode-e2e"
}

func linkerFlags(goos, stamp string) string {
	if goos == "windows" {
		return "-H=windowsgui " + stamp
	}
	return stamp
}

func testEnvironment(env []string, binary, artifacts string) []string {
	result := make([]string, 0, len(env)+2)
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "RCODE_E2E_BINARY") && !strings.EqualFold(key, "RCODE_E2E_ARTIFACTS") {
			result = append(result, entry)
		}
	}
	return append(result, "RCODE_E2E_BINARY="+binary, "RCODE_E2E_ARTIFACTS="+artifacts)
}

func checkoutRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		fields := strings.Fields(string(data))
		if err == nil && len(fields) >= 2 && fields[0] == "module" && fields[1] == "gcode" {
			return filepath.EvalSymlinks(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run from the GCode checkout")
		}
		dir = parent
	}
}

func evidenceDirectory(root, out string) (string, error) {
	if !filepath.IsAbs(out) {
		out = filepath.Join(root, out)
	}
	out = filepath.Clean(out)
	if !within(root, out) {
		return "", errors.New("evidence directory must stay in this checkout")
	}
	// Check existing ancestors before creating directories through any symlinks.
	ancestor := out
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			if !within(root, resolved) {
				return "", errors.New("evidence directory resolves outside this checkout")
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		ancestor = filepath.Dir(ancestor)
	}
	if err := os.MkdirAll(out, 0700); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(out)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
