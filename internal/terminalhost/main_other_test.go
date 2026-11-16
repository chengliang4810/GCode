//go:build !windows

package terminalhost

import (
	"testing"

	"gcode/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }
