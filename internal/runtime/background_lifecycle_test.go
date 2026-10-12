package runtime

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gcode/internal/background"
)

func TestEngineCloseOwnsPublishedSpecialistRuntime(t *testing.T) {
	engine, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("runtime.New returned error: %v", err)
	}

	shared, err := engine.specialistRuntime()
	if err != nil {
		t.Fatalf("specialistRuntime returned error: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if _, err := shared.Manager(); err == nil || !strings.Contains(err.Error(), "specialist runtime is closed") {
		t.Fatalf("published specialist runtime after Close = %v", err)
	}
	if _, err := engine.specialistRuntime(); err == nil || !strings.Contains(err.Error(), "runtime is closed") {
		t.Fatalf("specialistRuntime after Close = %v", err)
	}
}

func TestEngineClosePreventsLazySpecialistRuntimeCreation(t *testing.T) {
	engine, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("runtime.New returned error: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if _, err := engine.specialistRuntime(); err == nil || !strings.Contains(err.Error(), "runtime is closed") {
		t.Fatalf("specialistRuntime after lazy Close = %v", err)
	}
}

func TestEngineCloseConcurrentCallersShareSpecialistCloseResult(t *testing.T) {
	engine, err := New(Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("runtime.New returned error: %v", err)
	}
	specialistRuntime, err := engine.specialistRuntime()
	if err != nil {
		t.Fatalf("specialistRuntime returned error: %v", err)
	}
	manager, err := specialistRuntime.Manager()
	if err != nil {
		t.Fatalf("specialist Runtime.Manager returned error: %v", err)
	}
	const taskID = "ignored_engine_task"
	if _, err := manager.Register(background.RegisterInput{TaskID: taskID, Type: "specialist"}); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	_, cancel := context.WithCancel(context.Background())
	if err := specialistRuntime.StartTask(taskID, manager, cancel); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseFixture := func() { releaseOnce.Do(func() { close(release) }) }
	childDone := make(chan struct{})
	go func() {
		defer close(childDone)
		defer specialistRuntime.FinishTask(taskID)
		<-release
	}()
	t.Cleanup(func() {
		releaseFixture()
		select {
		case <-childDone:
		case <-time.After(time.Second):
			t.Error("unresponsive engine child fixture did not release")
		}
	})

	results := make(chan error, 2)
	go func() { results <- engine.Close() }()
	go func() { results <- engine.Close() }()
	firstErr := <-results
	secondErr := <-results
	if firstErr == nil || !strings.Contains(firstErr.Error(), "timed out") || !strings.Contains(firstErr.Error(), taskID) {
		t.Fatalf("first Engine.Close error = %v, want timeout naming %s", firstErr, taskID)
	}
	if secondErr == nil || secondErr.Error() != firstErr.Error() {
		t.Fatalf("concurrent Engine.Close error = %v, want same timeout result as %v", secondErr, firstErr)
	}

	releaseFixture()
	select {
	case <-childDone:
	case <-time.After(time.Second):
		t.Fatal("unresponsive engine child fixture did not finish after release")
	}
	if repeatedErr := engine.Close(); repeatedErr == nil || repeatedErr.Error() != firstErr.Error() {
		t.Fatalf("repeated Engine.Close error = %v, want same timeout result as %v", repeatedErr, firstErr)
	}
}
