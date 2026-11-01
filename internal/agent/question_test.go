package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestQuestionToolCollectsStructuredAnswers(t *testing.T) {
	asked := []Question{}
	tool := QuestionTool{Ask: func(_ context.Context, question Question) (string, error) {
		asked = append(asked, question)
		if question.MultiSelect {
			return "A, B", nil
		}
		return "Custom answer", nil
	}}
	raw := json.RawMessage(`{"questions":[{"question":"Choose features?","header":"Features","options":[{"label":"A","description":"Feature A"},{"label":"B","description":"Feature B"}],"multiSelect":true},{"question":"Choose mode?","header":"Mode","options":[{"label":"Fast","description":"Run quickly"},{"label":"Full","description":"Run fully"}]}]}`)
	out, err := tool.Execute(context.Background(), "", raw)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Questions []Question        `json:"questions"`
		Answers   map[string]string `json:"answers"`
	}
	if err := json.Unmarshal([]byte(out.Text), &result); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 2 || !asked[0].MultiSelect || asked[0].Options[1].Description != "Feature B" || result.Answers["Choose features?"] != "A, B" || result.Answers["Choose mode?"] != "Custom answer" {
		t.Fatalf("lost structured question/answer data: %+v %+v", asked, result)
	}
}

func TestQuestionToolValidatesAllQuestionsBeforeAsking(t *testing.T) {
	for _, test := range []struct{ name, raw string }{
		{"old contract", `{"title":"Old?","options":["A","B"]}`},
		{"empty", `{"questions":[]}`},
		{"duplicate options", `{"questions":[{"question":"Pick?","header":"Pick","options":[{"label":"A","description":"A"},{"label":"A","description":"B"}]}]}`},
		{"oversized header", `{"questions":[{"question":"Pick?","header":"1234567890123","options":[{"label":"A","description":"A"},{"label":"B","description":"B"}]}]}`},
		{"unknown option", `{"questions":[{"question":"Pick?","header":"Pick","options":[{"label":"A","description":"A","extra":true},{"label":"B","description":"B"}]}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			tool := QuestionTool{Ask: func(context.Context, Question) (string, error) { t.Fatal("asked invalid input"); return "", nil }}
			if _, err := tool.Execute(context.Background(), "", json.RawMessage(test.raw)); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
}

func TestQuestionToolPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tool := QuestionTool{Ask: func(context.Context, Question) (string, error) { t.Fatal("asked cancelled question"); return "", nil }}
	_, err := tool.Execute(ctx, "", json.RawMessage(`{"questions":[{"question":"Pick?","header":"Pick","options":[{"label":"A","description":"A"},{"label":"B","description":"B"}]}]}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
