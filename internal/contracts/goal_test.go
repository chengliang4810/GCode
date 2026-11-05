package contracts

import (
	"strings"
	"testing"
	"time"
)

func TestGoalEditValidationAndClone(t *testing.T) {
	command := Command{Kind: "goal.edit", SessionID: "session", ID: "goal",
		GoalEdit: &GoalEditInput{Objective: "Edited objective", ExpectedRevision: 7}}
	if err := command.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := command.Clone()
	cloned.GoalEdit.Objective = "changed by worker"
	if command.GoalEdit.Objective != "Edited objective" {
		t.Fatal("goal edit clone shares worker state")
	}
	for _, input := range []GoalEditInput{
		{Objective: " ", ExpectedRevision: 7}, {Objective: ""},
		{Objective: strings.Repeat("x", 4001), ExpectedRevision: 7},
	} {
		command.GoalEdit = &input
		if command.Validate() == nil {
			t.Fatal("invalid goal edit accepted:", input)
		}
	}
}

func TestGoalValidationAndCloneOwnership(t *testing.T) {
	zero, negative, positive := int64(0), int64(-1), int64(100)
	for _, input := range []GoalInput{
		{}, {Objective: "  "}, {Objective: strings.Repeat("目", 4001)},
		{Objective: "task", TokenBudget: &zero}, {Objective: "task", TokenBudget: &negative},
	} {
		if err := input.Validate(); err == nil {
			t.Fatalf("accepted invalid goal: %+v", input)
		}
	}
	input := GoalInput{Objective: strings.Repeat("目", 4000), TokenBudget: &positive}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	goal := &SessionGoal{
		Objective: "task", TokenBudget: &positive, ActiveSince: &now,
		Verifications: []GoalVerification{{Reason: "original"}},
	}
	snapshot := CloneSnapshot(Snapshot{Sessions: []Session{{Goal: goal}}})
	clone := snapshot.Sessions[0].Goal
	*clone.TokenBudget, *clone.ActiveSince, clone.Verifications[0].Reason = 1, now.Add(time.Hour), "changed"
	if *goal.TokenBudget != 100 || !goal.ActiveSince.Equal(now) || goal.Verifications[0].Reason != "original" {
		t.Fatal("snapshot shares mutable authoritative goal state")
	}
	command := (Command{Kind: "goal.set", SessionID: "session", Goal: &input}).Clone()
	*command.Goal.TokenBudget = 9
	if *input.TokenBudget != 100 {
		t.Fatal("goal command retained caller-owned budget")
	}
}
