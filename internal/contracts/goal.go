package contracts

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type GoalStatus string

const (
	GoalActive        GoalStatus = "active"
	GoalVerifying     GoalStatus = "verifying"
	GoalPaused        GoalStatus = "paused"
	GoalBudgetLimited GoalStatus = "budget_limited"
	GoalComplete      GoalStatus = "complete"
)

type GoalInput struct {
	InputText   string            `json:"inputText,omitempty"`
	References  []PromptReference `json:"references,omitempty"`
	Objective   string            `json:"objective"`
	TokenBudget *int64            `json:"tokenBudget,omitempty"`
}

type GoalEditInput struct {
	Objective        string `json:"objective"`
	ExpectedRevision uint64 `json:"expectedRevision"`
}

func (g GoalEditInput) Validate() error {
	if err := (GoalInput{Objective: g.Objective}).Validate(); err != nil {
		return err
	}
	return nil
}

func (g GoalInput) Validate() error {
	objective := strings.TrimSpace(g.Objective)
	if objective == "" || utf8.RuneCountInString(objective) > 4000 {
		return errors.New("goal objective must contain 1 to 4000 characters")
	}
	if g.TokenBudget != nil && *g.TokenBudget <= 0 {
		return errors.New("goal token budget must be positive")
	}
	return nil
}

type GoalVerification struct {
	Iteration  int       `json:"iteration"`
	Passed     bool      `json:"passed"`
	Reason     string    `json:"reason"`
	NextAction string    `json:"nextAction,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// SessionGoal is authoritative core state, not a projection of transcript text.
type SessionGoal struct {
	Instructions      string             `json:"instructions,omitempty"`
	ID                string             `json:"id"`
	InitialInputID    string             `json:"initialInputId"`
	Objective         string             `json:"objective"`
	Revision          uint64             `json:"revision"`
	Status            GoalStatus         `json:"status"`
	Iteration         int                `json:"iteration"`
	TokenBudget       *int64             `json:"tokenBudget,omitempty"`
	TokensUsed        int64              `json:"tokensUsed"`
	UnknownUsageCount int                `json:"unknownUsageCount"`
	TimeUsedMS        int64              `json:"timeUsedMs"`
	ActiveSince       *time.Time         `json:"activeSince,omitempty"`
	LastSeenAt        time.Time          `json:"lastSeenAt"`
	Reason            string             `json:"reason,omitempty"`
	NextAction        string             `json:"nextAction,omitempty"`
	Verifications     []GoalVerification `json:"verifications"`
	CreatedAt         time.Time          `json:"createdAt"`
	UpdatedAt         time.Time          `json:"updatedAt"`
}

func (g *SessionGoal) Clone() *SessionGoal {
	if g == nil {
		return nil
	}
	result := *g
	result.TokenBudget = copyPointer(g.TokenBudget)
	result.ActiveSince = copyPointer(g.ActiveSince)
	result.Verifications = slices.Clone(g.Verifications)
	return &result
}
