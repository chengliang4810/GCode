package workbench

import (
	"context"
	"errors"
	"sync"
	"time"
)

type scriptResult struct {
	value any
	err   error
}

type scriptTask struct {
	ctx    context.Context
	code   string
	key    string
	result chan scriptResult
}

// scriptQueue preserves the order of picker labels, marker removal and reset.
// It owns one cancellable worker per live browser, rather than one goroutine per
// hover or draft change. Coalescing keeps rapid hover updates bounded.
type scriptQueue struct {
	ctx    context.Context
	cancel context.CancelFunc
	page   pageEvaluator
	mu     sync.Mutex
	tasks  []scriptTask
	wake   chan struct{}
	done   chan struct{}
}

func newScriptQueue(ctx context.Context, page pageEvaluator) *scriptQueue {
	ctx, cancel := context.WithCancel(ctx)
	q := &scriptQueue{ctx: ctx, cancel: cancel, page: page, wake: make(chan struct{}, 1), done: make(chan struct{})}
	go q.run()
	return q
}

func (q *scriptQueue) enqueue(task scriptTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ctx.Err(); err != nil {
		return err
	}
	if task.key != "" {
		for i := range q.tasks {
			if q.tasks[i].key == task.key {
				q.tasks = append(q.tasks[:i], q.tasks[i+1:]...)
				break
			}
		}
	}
	if len(q.tasks) >= 128 {
		return errors.New("browser annotation script queue is full")
	}
	q.tasks = append(q.tasks, task)
	select {
	case q.wake <- struct{}{}:
	default:
	}
	return nil
}

func (q *scriptQueue) EvalContext(ctx context.Context, code string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	task := scriptTask{ctx: ctx, code: code, result: make(chan scriptResult, 1)}
	if err := q.enqueue(task); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-q.ctx.Done():
		return nil, q.ctx.Err()
	case result := <-task.result:
		return result.value, result.err
	}
}

func (q *scriptQueue) post(code, key string) error {
	return q.enqueue(scriptTask{ctx: q.ctx, code: code, key: key})
}

func (q *scriptQueue) close() { q.cancel() }

func (q *scriptQueue) run() {
	defer close(q.done)
	defer func() {
		q.mu.Lock()
		q.tasks = nil
		q.mu.Unlock()
	}()
	for {
		select {
		case <-q.ctx.Done():
			return
		case <-q.wake:
		}
		for {
			q.mu.Lock()
			if len(q.tasks) == 0 || q.ctx.Err() != nil {
				q.mu.Unlock()
				break
			}
			task := q.tasks[0]
			q.tasks[0] = scriptTask{}
			q.tasks = q.tasks[1:]
			q.mu.Unlock()
			ctx, cancel := context.WithTimeout(task.ctx, 2*time.Second)
			stop := context.AfterFunc(q.ctx, cancel)
			var result scriptResult
			if result.err = ctx.Err(); result.err == nil {
				result.value, result.err = q.page.EvalContext(ctx, task.code)
			}
			stop()
			cancel()
			if task.result != nil {
				task.result <- result
			}
		}
	}
}
