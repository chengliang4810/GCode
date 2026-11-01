package workflowprobe

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MoeForge/moejs"
)

func TestAsyncTypeScriptJSONReplay(t *testing.T) {
	const source = `
declare function ask<T>(site: string, input: string): Promise<T>;
type Reply = { value: number };
export async function run() {
  const seed = 5;
  const pair = await Promise.all([
    ask<Reply>("left", "2").then(item => item), ask<Reply>("right", "3")
  ]);
  let total = pair.reduce((n, item) => n + item.value, seed);
  for (let round = 0; round < 2; round++) {
    const next = await ask<Reply>("refine-" + round, String(total));
    total = next.value;
  }
  return { total, count: pair.length };
}`
	path := filepath.Join(t.TempDir(), "run.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	both := make(chan struct{})
	var inFlight, peak, started atomic.Int32
	var calls atomic.Int32
	result, err := run(ctx, source, path, func(ctx context.Context, site, input string) (any, error) {
		calls.Add(1)
		active := inFlight.Add(1)
		defer inFlight.Add(-1)
		for old := peak.Load(); active > old && !peak.CompareAndSwap(old, active); old = peak.Load() {
		}
		if site == "left" || site == "right" {
			if started.Add(1) == 2 {
				close(both)
			}
			select {
			case <-both:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		var value int
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			return nil, err
		}
		return map[string]int{"value": value + 1}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != `{"total":14,"count":2}` || peak.Load() != 2 || calls.Load() != 4 {
		t.Fatalf("result=%s peak=%d calls=%d", result, peak.Load(), calls.Load())
	}
	replayed, err := run(ctx, source, path, func(context.Context, string, string) (any, error) {
		return nil, errors.New("completed host work must not repeat")
	})
	if err != nil || string(replayed) != string(result) {
		t.Fatalf("replay=%s err=%v", replayed, err)
	}
	t.Logf("new runtime replayed JSON results; peak host concurrency=%d", peak.Load())
}

func TestRebuildRetriesOnlyUnfinishedNode(t *testing.T) {
	const source = `export async function run() {
  const base = await ask("base", "input");
  return await ask("tail", base);
}`
	path := filepath.Join(t.TempDir(), "run.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := run(ctx, source, path, func(_ context.Context, site, _ string) (any, error) {
		if site == "tail" {
			return nil, errors.New("simulate interrupted host work before result commit")
		}
		return "base-result", nil
	})
	if err == nil {
		t.Fatal("first run should stop without the tail result")
	}
	var sites []string
	result, err := run(ctx, source, path, func(_ context.Context, site, _ string) (any, error) {
		sites = append(sites, site)
		return "done", nil
	})
	if err != nil || string(result) != `"done"` || len(sites) != 1 || sites[0] != "tail" {
		t.Fatalf("result=%s sites=%v err=%v", result, sites, err)
	}
}

func TestNaiveJSONCacheDoesNotPreservePromiseRace(t *testing.T) {
	const source = `export async function run() {
  const winner = await Promise.race([ask("A", ""), ask("B", "")]);
  await ask("release", "");
  return winner;
}`
	path := filepath.Join(t.TempDir(), "run.json")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	both := make(chan struct{})
	var started atomic.Int32
	var aReleased sync.Once
	releaseA := make(chan struct{})
	result, err := run(ctx, source, path, func(ctx context.Context, site, _ string) (any, error) {
		if site == "release" {
			aReleased.Do(func() { close(releaseA) })
			return nil, nil
		}
		if started.Add(1) == 2 {
			close(both)
		}
		select {
		case <-both:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if site == "A" {
			select {
			case <-releaseA:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return site, nil
	})
	if err != nil || string(result) != `"B"` {
		t.Fatalf("original=%s err=%v", result, err)
	}
	replayed, err := run(ctx, source, path, func(context.Context, string, string) (any, error) {
		return nil, errors.New("all nodes should be cached")
	})
	if err != nil || string(replayed) != `"A"` {
		t.Fatalf("naive replay=%s err=%v", replayed, err)
	}
	t.Logf("original=%s replay=%s: stored results alone do not preserve settlement order", result, replayed)
}

func TestCompileTSErasesTypesWithoutChecking(t *testing.T) {
	const source = `export function run() { const n: number = "not-a-number"; return n; }`
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := run(ctx, source, filepath.Join(t.TempDir(), "run.json"), nil)
	if err != nil || string(result) != `"not-a-number"` {
		t.Fatalf("result=%s err=%v", result, err)
	}
	t.Log("invalid TS assignment compiled: schema/type checking needs a separate layer")
}

func TestDirectEvalCapturesLocalBinding(t *testing.T) {
	const source = `export function run() {
  const seed: number = 7;
  const filled = eval("((value) => seed + value)");
  return filled(5);
}`
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := run(ctx, source, filepath.Join(t.TempDir(), "run.json"), nil)
	if err != nil || string(result) != `12` {
		t.Fatalf("result=%s err=%v", result, err)
	}
}

func TestContextInterruptsInfiniteScript(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{})
	done := make(chan error, 1)
	path := filepath.Join(t.TempDir(), "run.json")
	go func() {
		_, err := run(ctx, `export function run() {
  for (let i = 0; ; i++) { if (i === 10000) ask("entered", ""); }
}`, path, func(context.Context, string, string) (any, error) {
			close(entered)
			return nil, nil
		})
		done <- err
	}()
	select {
	case <-entered:
		cancel()
	case err := <-done:
		t.Fatalf("script stopped before entering loop: %v", err)
	case <-ctx.Done():
		t.Fatal("script did not enter loop")
	}
	select {
	case err := <-done:
		var interrupted *moejs.InterruptedError
		if !errors.As(err, &interrupted) {
			t.Fatalf("expected runtime interrupt, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("script did not stop after cancellation")
	}
}

func TestJSONJournalRejectsChangedIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.json")
	j, err := openJournal(path, "script")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := j.admit("site", "input"); err != nil {
		t.Fatal(err)
	}
	if err := j.complete("site", json.RawMessage(`1`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := j.admit("site", "changed"); err == nil {
		t.Fatal("changed input must not hit completed cache")
	}
	if _, err := openJournal(path, "changed script"); err == nil {
		t.Fatal("changed script must not reuse journal")
	}
}
