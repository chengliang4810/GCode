package specialist

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gcode/internal/background"
)

type RuntimeOptions struct {
	Manager     *background.Manager
	ManagerFunc BackgroundManagerFunc
}

type Runtime struct {
	mu          sync.Mutex
	manager     *background.Manager
	managerFunc BackgroundManagerFunc
	promptFiles map[string]string
	tasks       map[string]struct{}
	tasksDone   chan struct{}
	closed      bool
	closeDone   chan struct{}
	closeErr    error
}

const runtimeCloseTimeout = 5 * time.Second

func NewRuntime(options RuntimeOptions) *Runtime {
	tasksDone := make(chan struct{})
	close(tasksDone)
	return &Runtime{
		manager:     options.Manager,
		managerFunc: options.ManagerFunc,
		promptFiles: map[string]string{},
		tasks:       map[string]struct{}{},
		tasksDone:   tasksDone,
	}
}

func (runtime *Runtime) Manager() (*background.Manager, error) {
	if runtime == nil {
		return nil, errors.New("specialist runtime is not configured")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return nil, errors.New("specialist runtime is closed")
	}
	if runtime.manager != nil {
		return runtime.manager, nil
	}
	managerFunc := runtime.managerFunc
	if managerFunc == nil {
		return nil, errors.New("background manager is not configured")
	}
	manager, err := managerFunc()
	if err != nil {
		return nil, err
	}
	runtime.manager = manager
	return manager, nil
}

// StartTask atomically publishes an in-process task owner and gives Engine.Close
// a waitable slot for the child. Holding runtime.mu while registering the owner
// closes the gap in which Close could observe a task without a cancellation
// function and then wait forever for it.
func (runtime *Runtime) StartTask(taskID string, manager *background.Manager, cancel func()) error {
	if runtime == nil {
		return nil
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("background task id is required")
	}
	if manager == nil {
		return errors.New("background task manager is required")
	}
	if cancel == nil {
		return errors.New("background task cancel function is required")
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.closed {
		return errors.New("specialist runtime is closed")
	}
	if _, exists := runtime.tasks[taskID]; exists {
		return fmt.Errorf("background task already running: %s", taskID)
	}
	if err := manager.RegisterCancel(taskID, cancel); err != nil {
		return err
	}
	if len(runtime.tasks) == 0 {
		runtime.tasksDone = make(chan struct{})
	}
	runtime.tasks[taskID] = struct{}{}
	return nil
}

// FinishTask releases the Engine/Service wait slot for an in-process child.
// It is safe to call after Runtime.Close or more than once for a task.
func (runtime *Runtime) FinishTask(taskID string) {
	if runtime == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	runtime.mu.Lock()
	if _, exists := runtime.tasks[taskID]; !exists {
		runtime.mu.Unlock()
		return
	}
	delete(runtime.tasks, taskID)
	if len(runtime.tasks) == 0 {
		if runtime.tasksDone != nil {
			close(runtime.tasksDone)
		}
		runtime.tasksDone = closedChannel()
	}
	runtime.mu.Unlock()
}

func (runtime *Runtime) TrackPromptFile(taskID string, promptFile string) {
	if runtime == nil || taskID == "" || promptFile == "" {
		return
	}
	runtime.mu.Lock()
	if runtime.closed {
		runtime.mu.Unlock()
		cleanupPromptFile(promptFile)
		return
	}
	defer runtime.mu.Unlock()
	runtime.promptFiles[taskID] = promptFile
}

func (runtime *Runtime) UntrackPromptFile(taskID string) {
	if runtime == nil || taskID == "" {
		return
	}
	promptFile := ""
	runtime.mu.Lock()
	if runtime.promptFiles != nil {
		promptFile = runtime.promptFiles[taskID]
		delete(runtime.promptFiles, taskID)
	}
	runtime.mu.Unlock()
	cleanupPromptFile(promptFile)
}

func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	var (
		manager   *background.Manager
		tasksDone <-chan struct{}
		closeDone chan struct{}
	)
	runtime.mu.Lock()
	if runtime.closed {
		closeDone = runtime.closeDone
		runtime.mu.Unlock()
		if closeDone != nil {
			<-closeDone
		}
		runtime.mu.Lock()
		err := runtime.closeErr
		runtime.mu.Unlock()
		return err
	}
	runtime.closed = true
	manager = runtime.manager
	closeDone = make(chan struct{})
	runtime.closeDone = closeDone
	tasksDone = runtime.tasksDone
	if tasksDone == nil {
		tasksDone = closedChannel()
	}
	runtime.mu.Unlock()

	var errs []error
	if manager != nil {
		if err := manager.KillRunning(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := runtime.waitForTasks(tasksDone); err != nil {
		errs = append(errs, err)
	} else {
		runtime.mu.Lock()
		promptFiles := make([]string, 0, len(runtime.promptFiles))
		for taskID, promptFile := range runtime.promptFiles {
			promptFiles = append(promptFiles, promptFile)
			delete(runtime.promptFiles, taskID)
		}
		runtime.mu.Unlock()
		for _, promptFile := range promptFiles {
			cleanupPromptFile(promptFile)
		}
	}

	closeErr := errors.Join(errs...)
	runtime.mu.Lock()
	runtime.closeErr = closeErr
	close(closeDone)
	runtime.mu.Unlock()
	return closeErr
}

func (runtime *Runtime) waitForTasks(tasksDone <-chan struct{}) error {
	timer := time.NewTimer(runtimeCloseTimeout)
	defer timer.Stop()
	select {
	case <-tasksDone:
		return nil
	case <-timer.C:
		taskIDs := runtime.runningTaskIDs()
		if len(taskIDs) == 0 {
			return nil
		}
		return fmt.Errorf("specialist runtime close timed out waiting for background tasks to finish: %s", strings.Join(taskIDs, ", "))
	}
}

func (runtime *Runtime) runningTaskIDs() []string {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	ids := make([]string, 0, len(runtime.tasks))
	for taskID := range runtime.tasks {
		ids = append(ids, taskID)
	}
	sort.Strings(ids)
	return ids
}

func closedChannel() chan struct{} {
	channel := make(chan struct{})
	close(channel)
	return channel
}
