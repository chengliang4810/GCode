package workflow

import (
	"sort"
	"time"

	"gcode/internal/contracts"
)

// 运行目录：一条对话里所有工作流运行的目录页。它是列表，不是第二张时间线，
// 也不解释运行内部发生了什么。

// RunDirectoryLimit is how many runs one directory page reads. It is the
// ceiling of what a conversation has as history, chosen once here so a footer
// count and the page itself cannot disagree.
const RunDirectoryLimit = 64

// RunDirectoryRow is one run in the directory, as much as a list row says.
type RunDirectoryRow struct {
	RunID       string
	Name        string
	Status      string
	StopReason  string
	StartedAt   time.Time
	FinishedAt  *time.Time
	UpdatedAt   time.Time
	Artifacts   int
	Phases      int
	Concurrency int
}

// RunDirectory is one conversation's runs, split the way a reader asks for
// them: the ones still moving, and the ones that ended.
type RunDirectory struct {
	// Running holds pending and running runs.
	Running []RunDirectoryRow
	// Ended holds every other run; the row's own status word says how it ended.
	Ended []RunDirectoryRow
	// Truncated reports that the page took its whole limit, so more may follow.
	// It counts what was read, not what survived filtering: a page of runs the
	// reader cannot open is still a full page.
	Truncated bool
}

// BuildRunDirectory projects one conversation's runs into its directory page.
// Newest first, in one place: two orderings of the same list would make rows
// jump whenever either side re-read.
func BuildRunDirectory(runs []contracts.WorkflowRun, sessionID string, limit int) RunDirectory {
	directory := RunDirectory{}
	if sessionID == "" {
		return directory
	}
	if limit <= 0 {
		limit = RunDirectoryLimit
	}
	mine := make([]contracts.WorkflowRun, 0, len(runs))
	for _, run := range runs {
		if run.SessionID == sessionID {
			mine = append(mine, run)
		}
	}
	directory.Truncated = len(mine) > limit
	sort.SliceStable(mine, func(i, j int) bool {
		if !mine[i].UpdatedAt.Equal(mine[j].UpdatedAt) {
			return mine[i].UpdatedAt.After(mine[j].UpdatedAt)
		}
		if !mine[i].StartedAt.Equal(mine[j].StartedAt) {
			return mine[i].StartedAt.After(mine[j].StartedAt)
		}
		return mine[i].ID > mine[j].ID
	})
	if len(mine) > limit {
		mine = mine[:limit]
	}
	for _, run := range mine {
		row := RunDirectoryRow{
			RunID: run.ID, Name: run.Name, Status: run.Status, StopReason: run.StopReason,
			StartedAt: run.StartedAt, FinishedAt: run.FinishedAt, UpdatedAt: run.UpdatedAt,
			Artifacts: len(run.Artifacts), Phases: len(run.Phases), Concurrency: NormalizeConcurrency(run.Concurrency),
		}
		if IsActive(run.Status) {
			directory.Running = append(directory.Running, row)
			continue
		}
		directory.Ended = append(directory.Ended, row)
	}
	return directory
}

// Count reports how many runs the page holds.
func (d RunDirectory) Count() int { return len(d.Running) + len(d.Ended) }

// Empty reports whether the conversation never ran a workflow.
func (d RunDirectory) Empty() bool { return d.Count() == 0 }
