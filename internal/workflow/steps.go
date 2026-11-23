package workflow

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// 脚本步骤：把一次运行自己的事件日志折成「按阶段分组」的步骤表。它只读事实，
// 不解释因果，也不改日志。

// Step 是面板上的一步。
type Step struct {
	Sequence uint64
	Kind     string
	At       time.Time
	// PhaseID is empty for facts about the run itself.
	PhaseID string
	// Detail is the one fact worth reading at a glance: a name, a title, an
	// answer, a reason. It is never the whole payload.
	Detail string
}

// StepGroup is one phase's steps, or the run's own steps when PhaseID is empty.
type StepGroup struct {
	PhaseID string
	Steps   []Step
}

// BuildSteps folds journal records into groups in sequence order. Groups keep
// the order their phase first appears, so the table reads like the run went.
func BuildSteps(records []JournalRecord) []StepGroup {
	groups := make([]StepGroup, 0, 4)
	index := map[string]int{}
	appendStep := func(phaseID string, step Step) {
		at, ok := index[phaseID]
		if !ok {
			at = len(groups)
			index[phaseID] = at
			groups = append(groups, StepGroup{PhaseID: phaseID})
		}
		groups[at].Steps = append(groups[at].Steps, step)
	}
	for _, record := range records {
		payload := stepPayloadOf(record.Data)
		phaseID := payload.PhaseID
		// 运行自己的事实不属于任何阶段；阶段事件本身属于它自己的阶段。
		if phaseID == "" && isPhaseFact(record.Kind) {
			phaseID = payload.PhaseID
		}
		appendStep(phaseID, Step{
			Sequence: record.Sequence,
			Kind:     record.Kind,
			At:       record.At,
			PhaseID:  phaseID,
			Detail:   stepDetail(record.Kind, payload),
		})
	}
	return groups
}

func isPhaseFact(kind string) bool {
	switch kind {
	case JournalPhaseEntered, JournalPhaseSettled, JournalPhaseReset:
		return true
	}
	return false
}

// stepPayload is the union of the payload fields a step shows. Decoding a log
// line into it must never fail because a field belongs to another kind.
type stepPayload struct {
	PhaseID     string `json:"phaseId"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	ActorID     string `json:"actorId"`
	Text        string `json:"text"`
	Answer      string `json:"answer"`
	Status      string `json:"status"`
	StopReason  string `json:"stopReason"`
	Error       string `json:"error"`
	ModelID     string `json:"modelId"`
	HasModel    bool   `json:"hasModel"`
	Concurrency int    `json:"concurrency"`
	HasConc     bool   `json:"hasConc"`
	SuccessorID string `json:"successorId"`
	Artifact    struct {
		Name string `json:"name"`
	} `json:"artifact"`
}

func stepPayloadOf(data json.RawMessage) stepPayload {
	var payload stepPayload
	if len(data) == 0 {
		return payload
	}
	_ = json.Unmarshal(data, &payload)
	return payload
}

// stepDetail is the short fact a step shows next to its kind.
func stepDetail(kind string, payload stepPayload) string {
	switch kind {
	case JournalPhaseEntered, JournalPhaseSettled, JournalPhaseReset:
		return firstText(payload.Name, payload.PhaseID)
	case JournalActorStarted, JournalActorSettled:
		return firstText(payload.Name, payload.ActorID)
	case JournalQuestionAsked:
		return payload.Text
	case JournalQuestionAnswer:
		return payload.Answer
	case JournalHoleWaiting:
		return payload.Title
	case JournalHoleFilled:
		return firstText(payload.Title, payload.Text)
	case JournalArtifact:
		return payload.Artifact.Name
	case JournalRunRetuned:
		parts := make([]string, 0, 2)
		if payload.HasModel && payload.ModelID != "" {
			parts = append(parts, payload.ModelID)
		}
		if payload.HasConc {
			parts = append(parts, strconv.Itoa(payload.Concurrency))
		}
		return strings.Join(parts, " ")
	case JournalRunStopped:
		return firstText(payload.StopReason, payload.Text)
	case JournalRunSettled:
		return firstText(payload.Error, payload.Status)
	}
	return ""
}

func firstText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
