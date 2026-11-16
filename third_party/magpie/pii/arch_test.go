package pii

import (
	"os"
	"reflect"
	"testing"
)

// noFullX86 holds fullX86 at have for the test.
func noFullX86(t *testing.T, have bool) {
	was := fullX86
	fullX86 = func() bool { return have }
	t.Cleanup(func() { fullX86 = was })
}

// withBase puts the stand-in as pf-serve-base too.
func withBase(t *testing.T) {
	t.Helper()
	b, err := os.ReadFile(exe("pf-serve"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe("pf-serve-base"), b, 0o755); err != nil {
		t.Fatal(err)
	}
}

// A CPU without AVX2, FMA or BMI2 ended pf-serve with SIGILL on the GPU
// and on the CPU (echo_yip on X): pf-serve-base runs there, on the CPU
// alone, since pf-serve-vulkan is built like pf-serve.
func TestBaseWhereTheCPULacksAVX2(t *testing.T) {
	log := fakeInstall(t, "ok", true)
	withBase(t)
	noFullX86(t, false)
	if _, err := Find([]string{"hi Alice"}); err != nil {
		t.Fatal(err)
	}
	if starts := fakeLines(t, log, "start"); !reflect.DeepEqual(starts, []string{"start pf-serve-base CPU"}) {
		t.Fatalf("starts = %q", starts)
	}
}

// A CPU with all of them never runs pf-serve-base; and a pf-serve from
// before pf-serve-base was built is run as it was.
func TestBaseOnlyWhereNeeded(t *testing.T) {
	t.Run("full CPU", func(t *testing.T) {
		log := fakeInstall(t, "novulkan", true)
		withBase(t)
		noFullX86(t, true)
		if _, err := Find([]string{"hi Alice"}); err != nil {
			t.Fatal(err)
		}
		if starts := fakeLines(t, log, "start"); !reflect.DeepEqual(starts, []string{"start pf-serve-vulkan MTL0", "start pf-serve CPU"}) {
			t.Fatalf("starts = %q", starts)
		}
	})
	t.Run("no base on disk", func(t *testing.T) {
		log := fakeInstall(t, "novulkan", true)
		noFullX86(t, false)
		if _, err := Find([]string{"hi Alice"}); err != nil {
			t.Fatal(err)
		}
		if starts := fakeLines(t, log, "start"); !reflect.DeepEqual(starts, []string{"start pf-serve-vulkan MTL0", "start pf-serve CPU"}) {
			t.Fatalf("starts = %q", starts)
		}
	})
}
