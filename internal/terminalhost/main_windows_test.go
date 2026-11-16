package terminalhost

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	"gcode/internal/testenv"
)

func TestMain(m *testing.M) {
	// Shell launchers can pass an inherited ignore-Ctrl+C flag to the test
	// process. Clear it before spawning the real ConPTY shell so the fixture
	// exercises terminal interrupts independently of its launcher's policy.
	reset := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")
	if ok, _, err := reset.Call(0, 0); ok == 0 {
		fmt.Fprintln(os.Stderr, "restore terminal test interrupts:", err)
		os.Exit(1)
	}
	testenv.Main(m)
}
