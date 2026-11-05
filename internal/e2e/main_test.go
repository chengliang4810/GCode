//go:build e2e

package e2e

import (
	"testing"

	"gcode/internal/testenv"
)

func TestMain(m *testing.M) { testenv.Main(m) }
