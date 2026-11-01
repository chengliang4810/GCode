package processutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestHiddenCommandHasNoConsoleAndRetainsOutput(t *testing.T) {
	for _, mode := range []string{"1", "nested"} {
		t.Run(mode, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestConsoleProbe$")
			command.Env = append(os.Environ(), "RCODE_TEST_CONSOLE_PROBE="+mode)
			HideConsole(command)
			output, err := command.CombinedOutput()
			if err != nil || strings.TrimSpace(string(output)) != "console=0" {
				t.Fatalf("hidden command: output=%q err=%v", output, err)
			}
		})
	}
}

func TestConsoleProbe(t *testing.T) {
	mode := os.Getenv("RCODE_TEST_CONSOLE_PROBE")
	if mode == "nested" {
		// Match build tools that launch further console programs without
		// setting process flags themselves.
		child := exec.Command(os.Args[0], "-test.run=^TestConsoleProbe$")
		child.Env = append(os.Environ(), "RCODE_TEST_CONSOLE_PROBE=1")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Run(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if mode != "1" && mode != "failure" {
		return
	}
	handle, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow").Call()
	fmt.Printf("console=%d\n", handle)
	if mode == "failure" {
		fmt.Fprintln(os.Stderr, "expected error")
		os.Exit(7)
	}
	os.Exit(0)
}

func TestHiddenCommandRetainsFailureStatusAndStderr(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=^TestConsoleProbe$")
	command.Env = append(os.Environ(), "RCODE_TEST_CONSOLE_PROBE=failure")
	HideConsole(command)
	output, err := command.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 7 || !strings.Contains(string(output), "expected error") ||
		!strings.Contains(string(output), "console=0") {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestHideConsolePreservesProcessSettings(t *testing.T) {
	command := exec.Command("unused")
	attributes := &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	command.SysProcAttr = attributes
	HideConsole(command)
	HideConsole(command)
	if command.SysProcAttr != attributes || attributes.CreationFlags != syscall.CREATE_NEW_PROCESS_GROUP|0x08000000 {
		t.Fatalf("process attributes replaced or lost: %+v", command.SysProcAttr)
	}
	if attributes.HideWindow {
		t.Fatal("console suppression must not hide a native GUI")
	}
}
