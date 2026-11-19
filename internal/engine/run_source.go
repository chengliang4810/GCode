package engine

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"time"

	"gcode/internal/contracts"
)

const runSourceTimeout = 3 * time.Second

// Observe only execution provenance, without loading the ref/worktree picker.
// Git's documented exit-1 states are valid for detached/unborn HEAD; other
// failures must not clear good metadata and advance its observation time.
func inspectRunSource(ctx context.Context, directory string) (*gitRepository, error) {
	root, err := gitOutput(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	branch, err := gitOutput(ctx, directory, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil && !gitSourceExitOne(err) {
		return nil, err
	}
	head, headErr := gitOutput(ctx, directory, "rev-parse", "--verify", "--quiet", "HEAD")
	if headErr != nil {
		if branch == "" || !gitSourceExitOne(headErr) {
			return nil, headErr
		}
		// A missing branch ref is a real unborn branch, not a failed HEAD probe
		// in an otherwise committed checkout.
		if _, err := gitOutput(ctx, directory, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); !gitSourceExitOne(err) {
			return nil, headErr
		}
	}
	return &gitRepository{root: filepath.Clean(root), branch: branch, head: head}, nil
}

func gitSourceExitOne(err error) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 1
}

// Observe the actual execution directory on the run worker, never while holding
// the view lock. A failed probe keeps the previous observation and its old time;
// it must not turn stale metadata into a seemingly fresh observation.
func (e *Engine) refreshRunSource(ctx context.Context, session contracts.Session, run *activeRun,
	inspect func(context.Context, string) (*gitRepository, error),
) error {
	ctx, cancel := context.WithTimeout(ctx, runSourceTimeout)
	defer cancel()
	if ctx.Err() != nil {
		return nil
	}
	started := time.Now()
	repo, err := inspect(ctx, session.CWD)
	if err != nil || ctx.Err() != nil || repo == nil || repo.root == "" || (repo.branch == "" && repo.head == "") {
		return nil
	}
	observed := sourceContext(repo, session.CWD, session.WorktreeID)

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || ctx.Err() != nil || e.runs[session.ID] != run {
		return nil
	}
	index := e.sessionIndex(session.ID)
	if index < 0 {
		return nil
	}
	current := &e.state.Sessions[index]
	if !sameRecordedPath(current.CWD, session.CWD) || current.WorktreeID != session.WorktreeID ||
		(current.SourceContext != nil && current.SourceContext.ObservedAt.After(started)) {
		// A newer run, checkout selection or explicit Git refresh owns the view.
		return nil
	}
	priorBranch, priorSource := current.Branch, current.SourceContext
	current.Branch, current.SourceContext = repo.branch, observed
	e.markSessionDirtyLocked(session.ID)
	if err := e.persistLocked(session.ID); err != nil {
		e.state.Sessions[index].Branch, e.state.Sessions[index].SourceContext = priorBranch, priorSource
		return err
	}
	e.publishLocked(contracts.Event{Kind: "session.updated", SessionID: session.ID, RunID: run.id})
	return nil
}
