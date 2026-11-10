package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gcode/internal/model"
)

// RunToolOptions contains process-local settings owned by the execution host.
type RunToolOptions struct {
	Env        map[string]string
	EnvFile    string
	OpenOutput func() (io.WriteCloser, string, error)
}

const (
	maxActiveBackgroundTasks    = 64
	maxCompletedBackgroundTasks = 64
)

type runTasks struct {
	ctx        context.Context
	cancel     context.CancelFunc
	cwd        string
	env        map[string]string
	openOutput func() (io.WriteCloser, string, error)
	mu         sync.Mutex
	closed     bool
	tasks      map[string]*backgroundTask
	active     int
	completed  []string
	wg         sync.WaitGroup
}

type backgroundTask struct {
	id, command, description string
	cmd                      *exec.Cmd
	cancel                   context.CancelFunc
	done                     chan struct{}
	output, stdout, stderr   boundedOutput
	mu                       sync.Mutex
	status                   string
	exitCode                 int
	err                      error
}

// RunToolSet gives an execution owner its own background task namespace. Cleanup
// cancels and joins only processes started by this set; it is safe to repeat.
// At most 64 tasks run concurrently; the latest 64 completed results remain queryable.
func RunToolSet(ctx context.Context, cwd string, options ...RunToolOptions) ([]Tool, func()) {
	ownerCtx, cancel := context.WithCancel(ctx)
	owner := &runTasks{ctx: ownerCtx, cancel: cancel, cwd: cwd, env: map[string]string{}, tasks: map[string]*backgroundTask{}}
	for _, option := range options {
		for key, value := range option.Env {
			owner.env[key] = value
		}
		if option.EnvFile != "" {
			owner.env["RCODE_SESSION_ENV_FILE"] = option.EnvFile
		}
		if option.OpenOutput != nil {
			owner.openOutput = option.OpenOutput
		}
	}
	tools := CoreTools()
	for i, tool := range tools {
		if tool.Definition().Name == "Bash" {
			tools[i] = runBashTool{owner: owner, base: tool}
		}
	}
	tools = append(tools, taskOutputTool{owner}, taskStopTool{owner})
	var once sync.Once
	return tools, func() {
		once.Do(func() {
			owner.mu.Lock()
			owner.closed = true
			owner.cancel()
			owner.mu.Unlock()
			owner.wg.Wait()
		})
	}
}

type runBashTool struct {
	owner *runTasks
	base  Tool
}

func (t runBashTool) Concurrent() bool { return false }
func (t runBashTool) Definition() model.Tool {
	def := t.base.Definition()
	def.Parameters = maps.Clone(def.Parameters)
	def.Parameters["properties"] = maps.Clone(def.Parameters["properties"].(map[string]any))
	def.Description = strings.ReplaceAll(def.Description, "Background execution and sandbox overrides return explicit unsupported errors.", "run_in_background starts an owned task; use TaskOutput and TaskStop with its ID. Sandbox overrides return explicit unsupported errors.")
	def.Parameters["properties"].(map[string]any)["run_in_background"] = map[string]any{"type": "boolean", "description": "Run asynchronously and return a background task ID"}
	return def
}
func (t runBashTool) Execute(ctx context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	var args struct {
		Command        string `json:"command"`
		Description    string `json:"description"`
		Timeout        *int   `json:"timeout"`
		Background     bool   `json:"run_in_background"`
		DisableSandbox bool   `json:"dangerouslyDisableSandbox"`
	}
	if err := decodeSearchInput(raw, &args); err != nil {
		return model.ToolOutput{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	if err := t.owner.ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	if args.DisableSandbox {
		return model.ToolOutput{}, errors.New("dangerouslyDisableSandbox is not supported by this host")
	}
	timeout := 120000
	if args.Timeout != nil {
		timeout = *args.Timeout
	}
	if timeout < 1 || timeout > 600000 {
		return model.ToolOutput{}, errors.New("timeout must be 1..600000 milliseconds")
	}
	if !args.Background {
		callCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(t.owner.ctx, cancel)
		defer stop()
		return RunShell(callCtx, t.owner.cwd, "bash", args.Command, t.owner.env, time.Duration(timeout)*time.Millisecond, t.owner.openOutput)
	}
	task, err := t.owner.start(args.Command, args.Description, time.Duration(timeout)*time.Millisecond)
	if err != nil {
		return model.ToolOutput{}, err
	}
	data := map[string]any{"stdout": "", "stderr": "", "interrupted": false, "backgroundTaskId": task.id}
	return model.ToolOutput{Text: "Background task started: " + task.id, Data: data}, nil
}

func (r *runTasks) start(command, description string, timeout time.Duration) (*backgroundTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, errors.New("task owner is closed")
	}
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if r.active >= maxActiveBackgroundTasks {
		return nil, errors.New("this task owner has reached its 64 active background task limit")
	}
	token := make([]byte, 12)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	taskCtx, cancel := context.WithTimeout(r.ctx, timeout)
	cmd, err := newShellCommand(taskCtx, r.cwd, "bash", command, r.env)
	if err != nil {
		cancel()
		return nil, err
	}
	task := &backgroundTask{id: "task_" + hex.EncodeToString(token), command: command, description: description, cmd: cmd, cancel: cancel, done: make(chan struct{}), status: "running", exitCode: -1}
	task.output.openOutput = r.openOutput
	cmd.Stdout = io.MultiWriter(&task.output, &task.stdout)
	cmd.Stderr = io.MultiWriter(&task.output, &task.stderr)
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	r.tasks[task.id] = task
	r.active++
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer close(task.done)
		defer cancel()
		err := cmd.Wait()
		err = errors.Join(err, task.output.Close())
		task.mu.Lock()
		task.exitCode = cmd.ProcessState.ExitCode()
		task.err = err
		switch {
		case taskCtx.Err() != nil:
			task.status = "killed"
			task.err = taskCtx.Err()
		case err != nil:
			task.status = "failed"
		default:
			task.status = "completed"
		}
		task.mu.Unlock()
		r.mu.Lock()
		r.active--
		r.completed = append(r.completed, task.id)
		if len(r.completed) > maxCompletedBackgroundTasks {
			delete(r.tasks, r.completed[0])
			r.completed = r.completed[1:]
		}
		r.mu.Unlock()
	}()
	return task, nil
}

func (r *runTasks) find(id string) (*backgroundTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task := r.tasks[id]
	if task == nil {
		return nil, fmt.Errorf("task %q is not owned by this execution owner or its completed result has expired", id)
	}
	return task, nil
}

func (t *backgroundTask) snapshot() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	data := map[string]any{"task_id": t.id, "task_type": "local_bash", "status": t.status, "command": t.command, "description": t.description, "output": t.output.String()}
	if t.status != "running" {
		data["exit_code"] = t.exitCode
	}
	if t.err != nil {
		data["error"] = t.err.Error()
	}
	return data
}

type taskOutputTool struct{ owner *runTasks }

func (taskOutputTool) Concurrent() bool { return true }
func (taskOutputTool) Definition() model.Tool {
	return model.Tool{Name: "TaskOutput", Description: "Read output from an owned background task, optionally waiting for completion. The latest 64 completed results are retained; older completed IDs expire.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"task_id": map[string]any{"type": "string"}, "block": map[string]any{"type": "boolean", "default": true}, "timeout": map[string]any{"type": "integer", "minimum": 0, "maximum": 600000, "default": 30000}}, "required": []string{"task_id"}, "additionalProperties": false}}
}
func (t taskOutputTool) Execute(ctx context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	var args struct {
		ID      string `json:"task_id"`
		Block   *bool  `json:"block"`
		Timeout *int   `json:"timeout"`
	}
	if err := decodeSearchInput(raw, &args); err != nil {
		return model.ToolOutput{}, err
	}
	timeout := 30000
	if args.Timeout != nil {
		timeout = *args.Timeout
	}
	if timeout < 0 || timeout > 600000 {
		return model.ToolOutput{}, errors.New("timeout must be 0..600000 milliseconds")
	}
	task, err := t.owner.find(args.ID)
	if err != nil {
		return model.ToolOutput{}, err
	}
	block := args.Block == nil || *args.Block
	retrieval := "not_ready"
	if block {
		timer := time.NewTimer(time.Duration(timeout) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return model.ToolOutput{}, ctx.Err()
		case <-task.done:
			retrieval = "success"
		case <-timer.C:
			retrieval = "timeout"
		}
	} else {
		select {
		case <-task.done:
			retrieval = "success"
		default:
		}
	}
	data := map[string]any{"retrieval_status": retrieval, "task": task.snapshot()}
	output, err := json.Marshal(data)
	return model.ToolOutput{Text: string(output), Data: data, OutputFile: task.output.OutputPath()}, err
}

type taskStopTool struct{ owner *runTasks }

func (taskStopTool) Concurrent() bool { return true }
func (taskStopTool) Definition() model.Tool {
	return model.Tool{Name: "TaskStop", Description: "Stop an owned background task and wait for its process tree to exit.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"task_id": map[string]any{"type": "string"}}, "required": []string{"task_id"}, "additionalProperties": false}}
}
func (t taskStopTool) Execute(ctx context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	var args struct {
		ID string `json:"task_id"`
	}
	if err := decodeSearchInput(raw, &args); err != nil {
		return model.ToolOutput{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	task, err := t.owner.find(args.ID)
	if err != nil {
		return model.ToolOutput{}, err
	}
	task.cancel()
	select {
	case <-ctx.Done():
		return model.ToolOutput{}, ctx.Err()
	case <-task.done:
	}
	data := map[string]any{"task_id": task.id, "message": "Task stopped"}
	output, err := json.Marshal(data)
	return model.ToolOutput{Text: string(output), Data: data}, err
}
