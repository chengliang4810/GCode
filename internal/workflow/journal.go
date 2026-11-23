package workflow

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gcode/internal/contracts"
)

// The journal is the durable fact log of a run. Resume rebuilds a run from it
// instead of from a memory snapshot, so every state change that matters is
// appended before the UI is told about it.
const (
	JournalRunStarted     = "run.started"
	JournalPhaseEntered   = "phase.entered"
	JournalPhaseSettled   = "phase.settled"
	JournalPhaseReset     = "phase.reset"
	JournalActorStarted   = "actor.started"
	JournalActorSettled   = "actor.settled"
	JournalQuestionAsked  = "question.asked"
	JournalQuestionAnswer = "question.resolved"
	JournalHoleWaiting    = "hole.waiting"
	JournalHoleFilled     = "hole.filled"
	JournalArtifact       = "artifact.published"
	JournalRunRetuned     = "run.retuned"
	JournalRunStopped     = "run.stopped"
	JournalRunResumed     = "run.resumed"
	JournalRunSettled     = "run.settled"
)

// JournalRecord is one appended fact.
type JournalRecord struct {
	Sequence uint64          `json:"sequence"`
	Kind     string          `json:"kind"`
	At       time.Time       `json:"at"`
	RunID    string          `json:"runId"`
	Data     json.RawMessage `json:"data,omitempty"`
}

// RunStartedPayload carries the facts a replay needs to rebuild a run.
type RunStartedPayload struct {
	DefinitionID string                       `json:"definitionId,omitempty"`
	Name         string                       `json:"name"`
	SessionID    string                       `json:"sessionId,omitempty"`
	MessageID    string                       `json:"messageId,omitempty"`
	ProjectID    string                       `json:"projectId,omitempty"`
	HostID       string                       `json:"hostId,omitempty"`
	ModelID      string                       `json:"modelId,omitempty"`
	Concurrency  int                          `json:"concurrency"`
	Program      contracts.WorkflowProgram    `json:"program"`
	Args         []contracts.WorkflowArgValue `json:"args,omitempty"`
	Script       string                       `json:"script,omitempty"`
	Origin       string                       `json:"origin,omitempty"`
}

// PhasePayload records a phase entering or settling.
type PhasePayload struct {
	PhaseID   string     `json:"phaseId"`
	Name      string     `json:"name,omitempty"`
	Index     int        `json:"index"`
	Status    string     `json:"status,omitempty"`
	EnteredAt *time.Time `json:"enteredAt,omitempty"`
	SettledAt *time.Time `json:"settledAt,omitempty"`
	Rounds    int        `json:"rounds,omitempty"`
}

// ActorPayload records one instance starting or settling.
type ActorPayload struct {
	ActorID    string     `json:"actorId"`
	PhaseID    string     `json:"phaseId"`
	Name       string     `json:"name,omitempty"`
	ModelID    string     `json:"modelId,omitempty"`
	SessionID  string     `json:"sessionId,omitempty"`
	Status     string     `json:"status"`
	Error      string     `json:"error,omitempty"`
	Result     string     `json:"result,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// QuestionPayload records a question being asked or answered.
type QuestionPayload struct {
	QuestionID string     `json:"questionId"`
	ActorID    string     `json:"actorId,omitempty"`
	Text       string     `json:"text"`
	Context    string     `json:"context,omitempty"`
	Answer     string     `json:"answer,omitempty"`
	AskedAt    *time.Time `json:"askedAt,omitempty"`
	ResolvedAt *time.Time `json:"resolvedAt,omitempty"`
}

// HolePayload records a hole waiting to be filled and the fill that settled it.
type HolePayload struct {
	HoleID    string     `json:"holeId"`
	PhaseID   string     `json:"phaseId,omitempty"`
	ParentID  string     `json:"parentId,omitempty"`
	Title     string     `json:"title"`
	Text      string     `json:"text,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	FilledAt  *time.Time `json:"filledAt,omitempty"`
}

// ArtifactPayload records a published artifact.
type ArtifactPayload struct {
	Artifact contracts.WorkflowArtifact `json:"artifact"`
}

// RetunePayload records a settings change applied to a live run.
type RetunePayload struct {
	ModelID     string `json:"modelId,omitempty"`
	HasModel    bool   `json:"hasModel,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
	HasConc     bool   `json:"hasConc,omitempty"`
	StopReason  string `json:"stopReason,omitempty"`
	SuccessorID string `json:"successorId,omitempty"`
}

// SettlePayload records how a run ended.
type SettlePayload struct {
	Status     string     `json:"status"`
	StopReason string     `json:"stopReason,omitempty"`
	Error      string     `json:"error,omitempty"`
	Result     string     `json:"result,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// AppendRecords appends facts to a run's journal. The file is JSON Lines: one
// record per line, so a torn tail costs the last line and not the whole log.
func AppendRecords(path string, records []JournalRecord) error {
	if path == "" {
		return errors.New("journal path is required")
	}
	if len(records) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(line, '\n')); err != nil {
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

// ReadJournal reads a journal. Damaged lines are counted and skipped so one
// broken record cannot hide the facts around it.
func ReadJournal(path string) (records []JournalRecord, damaged int, err error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record JournalRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			damaged++
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return records, damaged, err
	}
	return records, damaged, nil
}

// JournalPath is the journal file of a run inside a storage directory.
func JournalPath(root, runID string) string {
	return filepath.Join(root, "workflows", runID+".jsonl")
}

// EncodePayload marshals a journal payload.
func EncodePayload(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode journal payload: %w", err)
	}
	return json.RawMessage(data), nil
}
