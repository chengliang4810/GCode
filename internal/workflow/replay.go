package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"gcode/internal/contracts"
)

// Replay rebuilds a run from its journal. Resume rebuilds the script runtime
// too and replays the settled facts; the projection here is the authoritative
// run record those facts produce.
func Replay(records []JournalRecord) (contracts.WorkflowRun, error) {
	var run contracts.WorkflowRun
	var started bool
	for _, record := range records {
		if record.Kind == JournalRunStarted {
			started = true
		}
		if !started {
			continue
		}
		if err := applyRecord(&run, record); err != nil {
			return contracts.WorkflowRun{}, fmt.Errorf("%s: %w", record.Kind, err)
		}
	}
	if !started {
		return contracts.WorkflowRun{}, errors.New("journal has no run.start record")
	}
	return run, nil
}

func applyRecord(run *contracts.WorkflowRun, record JournalRecord) error {
	switch record.Kind {
	case JournalRunStarted:
		var payload RunStartedPayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		run.ID = record.RunID
		run.DefinitionID = payload.DefinitionID
		run.Name = payload.Name
		run.SessionID = payload.SessionID
		run.MessageID = payload.MessageID
		run.ProjectID = payload.ProjectID
		run.HostID = payload.HostID
		run.ModelID = payload.ModelID
		run.Concurrency = NormalizeConcurrency(payload.Concurrency)
		run.Args = slices.Clone(payload.Args)
		run.Script = payload.Script
		run.Origin = payload.Origin
		run.Status = contracts.WorkflowStatusPending
		run.StartedAt = record.At
		run.UpdatedAt = record.At
		run.Phases = make([]contracts.WorkflowPhase, len(payload.Program.Phases))
		for i, phase := range payload.Program.Phases {
			run.Phases[i] = contracts.WorkflowPhase{
				ID:     phase.ID,
				Name:   phase.Name,
				Index:  i,
				Status: contracts.WorkflowPhasePending,
			}
			run.Edges = append(run.Edges, phase.Edges...)
		}
	case JournalPhaseEntered:
		var payload PhasePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		i := phaseIndex(run, payload.PhaseID)
		if i < 0 {
			return fmt.Errorf("unknown phase %q", payload.PhaseID)
		}
		run.Phases[i].Status = contracts.WorkflowPhaseRunning
		if payload.Name != "" {
			run.Phases[i].Name = payload.Name
		}
		run.Phases[i].EnteredAt = payload.EnteredAt
		run.Status = contracts.WorkflowStatusRunning
		run.UpdatedAt = record.At
	case JournalPhaseSettled:
		var payload PhasePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		i := phaseIndex(run, payload.PhaseID)
		if i < 0 {
			return fmt.Errorf("unknown phase %q", payload.PhaseID)
		}
		if payload.Status != "" {
			run.Phases[i].Status = payload.Status
		}
		run.Phases[i].SettledAt = payload.SettledAt
		run.Phases[i].Rounds = payload.Rounds
		observed, settled := phaseActors(run.Actors, payload.PhaseID)
		run.Phases[i].Observed, run.Phases[i].Settled = observed, settled
		run.UpdatedAt = record.At
	case JournalPhaseReset:
		var payload PhasePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		from := phaseIndex(run, payload.PhaseID)
		if from < 0 {
			return fmt.Errorf("unknown phase %q", payload.PhaseID)
		}
		// 回边让目标之后的阶段重新变成待执行；轮次保留，上限照样生效。
		for i := from; i < len(run.Phases); i++ {
			run.Phases[i].Status = contracts.WorkflowPhasePending
			run.Phases[i].EnteredAt, run.Phases[i].SettledAt = nil, nil
			run.Phases[i].Observed, run.Phases[i].Settled = 0, 0
		}
		run.Status = contracts.WorkflowStatusRunning
		run.UpdatedAt = record.At
	case JournalActorStarted:
		var payload ActorPayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		actor := ensureActor(run, payload.ActorID, payload.PhaseID)
		actor.Name = payload.Name
		actor.ModelID = payload.ModelID
		actor.SessionID = payload.SessionID
		actor.Status = contracts.WorkflowActorRunning
		actor.StartedAt = payload.StartedAt
		refreshPhaseCounts(run, payload.PhaseID)
		run.Status = contracts.WorkflowStatusRunning
		run.UpdatedAt = record.At
	case JournalActorSettled:
		var payload ActorPayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		actor := ensureActor(run, payload.ActorID, payload.PhaseID)
		actor.Status = ActorState(payload.Status)
		actor.Error = payload.Error
		actor.Result = payload.Result
		actor.FinishedAt = payload.FinishedAt
		refreshPhaseCounts(run, payload.PhaseID)
		run.UpdatedAt = record.At
	case JournalQuestionAsked:
		var payload QuestionPayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		run.Questions = append(run.Questions, contracts.WorkflowQuestion{
			ID:      payload.QuestionID,
			RunID:   run.ID,
			ActorID: payload.ActorID,
			Text:    payload.Text,
			Context: payload.Context,
			Status:  "open",
			AskedAt: timeValue(payload.AskedAt, record.At),
		})
		setActorState(run, payload.ActorID, contracts.WorkflowActorAsking)
		run.UpdatedAt = record.At
	case JournalQuestionAnswer:
		var payload QuestionPayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		for i := range run.Questions {
			if run.Questions[i].ID != payload.QuestionID {
				continue
			}
			run.Questions[i].Status = "resolved"
			run.Questions[i].Answer = payload.Answer
			run.Questions[i].ResolvedAt = payload.ResolvedAt
			if run.Questions[i].ActorID != "" {
				setActorState(run, run.Questions[i].ActorID, contracts.WorkflowActorRunning)
			}
		}
		run.UpdatedAt = record.At
	case JournalHoleWaiting:
		var payload HolePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		run.Holes = append(run.Holes, contracts.WorkflowHole{
			ID:        payload.HoleID,
			RunID:     run.ID,
			PhaseID:   payload.PhaseID,
			ParentID:  payload.ParentID,
			Title:     payload.Title,
			Status:    contracts.WorkflowHoleWaiting,
			CreatedAt: timeValue(payload.CreatedAt, record.At),
		})
		run.UpdatedAt = record.At
	case JournalHoleFilled:
		var payload HolePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		for i := range run.Holes {
			if run.Holes[i].ID != payload.HoleID {
				continue
			}
			run.Holes[i].Status = contracts.WorkflowHoleFilled
			run.Holes[i].FilledAt = payload.FilledAt
		}
		if payload.PhaseID != "" && phaseIndex(run, payload.PhaseID) < 0 {
			run.Phases = append(run.Phases, contracts.WorkflowPhase{
				ID:     payload.PhaseID,
				Name:   payload.Title,
				Index:  len(run.Phases),
				Status: contracts.WorkflowPhaseRunning,
			})
		}
		run.UpdatedAt = record.At
	case JournalArtifact:
		var payload ArtifactPayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		payload.Artifact.RunID = run.ID
		run.Artifacts = append(run.Artifacts, payload.Artifact)
		run.UpdatedAt = record.At
	case JournalRunRetuned:
		var payload RetunePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		if payload.HasConc {
			run.Concurrency = NormalizeConcurrency(payload.Concurrency)
		}
		if payload.HasModel {
			run.ModelID = payload.ModelID
		}
		run.UpdatedAt = record.At
	case JournalRunStopped:
		var payload SettlePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		run.Status = contracts.WorkflowStatusStopped
		run.StopReason = payload.StopReason
		run.UpdatedAt = record.At
	case JournalRunResumed:
		var payload SettlePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		run.Status = contracts.WorkflowStatusPending
		run.StopReason, run.Resumable = "", false
		run.FinishedAt = nil
		run.UpdatedAt = record.At
	case JournalRunSettled:
		var payload SettlePayload
		if err := json.Unmarshal(record.Data, &payload); err != nil {
			return err
		}
		run.Status = payload.Status
		run.StopReason = payload.StopReason
		run.Error = payload.Error
		run.Result = payload.Result
		run.FinishedAt = payload.FinishedAt
		run.Resumable = payload.Status == contracts.WorkflowStatusStopped &&
			payload.StopReason != contracts.WorkflowStopSuperseded
		run.UpdatedAt = record.At
	}
	return nil
}

func timeValue(value *time.Time, fallback time.Time) time.Time {
	if value == nil {
		return fallback
	}
	return *value
}

func phaseIndex(run *contracts.WorkflowRun, id string) int {
	return slices.IndexFunc(run.Phases, func(phase contracts.WorkflowPhase) bool { return phase.ID == id })
}

func phaseActors(actors []contracts.WorkflowActor, phaseID string) (observed, settled int) {
	for _, actor := range actors {
		if actor.PhaseID != phaseID {
			continue
		}
		observed++
		if state := ActorState(actor.Status); state == contracts.WorkflowActorDone || state == contracts.WorkflowActorFailed {
			settled++
		}
	}
	return observed, settled
}

func refreshPhaseCounts(run *contracts.WorkflowRun, phaseID string) {
	i := phaseIndex(run, phaseID)
	if i < 0 {
		return
	}
	run.Phases[i].Observed, run.Phases[i].Settled = phaseActors(run.Actors, phaseID)
}

func ensureActor(run *contracts.WorkflowRun, actorID, phaseID string) *contracts.WorkflowActor {
	for i := range run.Actors {
		if run.Actors[i].ID == actorID {
			return &run.Actors[i]
		}
	}
	run.Actors = append(run.Actors, contracts.WorkflowActor{ID: actorID, PhaseID: phaseID})
	return &run.Actors[len(run.Actors)-1]
}

func setActorState(run *contracts.WorkflowRun, actorID, state string) {
	for i := range run.Actors {
		if run.Actors[i].ID == actorID {
			run.Actors[i].Status = state
		}
	}
}
