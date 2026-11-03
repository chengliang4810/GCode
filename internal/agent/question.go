package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

type QuestionTool struct {
	Ask func(context.Context, []contracts.QuestionPage) ([]contracts.QuestionAnswer, error)
}

func (t QuestionTool) Definition() model.Tool {
	text := map[string]any{"type": "string"}
	option := map[string]any{"type": "object", "properties": map[string]any{"label": text, "description": text, "preview": text}, "required": []string{"label", "description"}, "additionalProperties": false}
	question := map[string]any{"type": "object", "properties": map[string]any{"question": text, "header": map[string]any{"type": "string", "maxLength": 12}, "options": map[string]any{"type": "array", "minItems": 2, "maxItems": 4, "items": option}, "multiSelect": map[string]any{"type": "boolean", "default": false}}, "required": []string{"question", "header", "options"}, "additionalProperties": false}
	return model.Tool{Name: "AskUserQuestion", Description: "Ask one to four questions and wait for the user's answers. Each question offers two to four options; the user may also enter a custom answer. multiSelect allows multiple choices.", Parameters: map[string]any{"type": "object", "properties": map[string]any{"questions": map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "items": question}}, "required": []string{"questions"}, "additionalProperties": false}}
}
func (t QuestionTool) Concurrent() bool { return false }
func (t QuestionTool) Execute(ctx context.Context, _ string, raw json.RawMessage) (model.ToolOutput, error) {
	var input struct {
		Questions []contracts.QuestionPage `json:"questions"`
	}
	if err := decodeSearchInput(raw, &input); err != nil {
		return model.ToolOutput{}, err
	}
	if len(input.Questions) < 1 || len(input.Questions) > 4 {
		return model.ToolOutput{}, errors.New("questions must contain 1..4 questions")
	}
	seen := map[string]bool{}
	for _, q := range input.Questions {
		if strings.TrimSpace(q.Question) == "" || strings.TrimSpace(q.Header) == "" || utf8.RuneCountInString(q.Header) > 12 || len(q.Options) < 2 || len(q.Options) > 4 || seen[q.Question] {
			return model.ToolOutput{}, errors.New("invalid or duplicate question: require question, header up to 12 characters, and 2..4 options")
		}
		seen[q.Question] = true
		labels := map[string]bool{}
		for _, option := range q.Options {
			if strings.TrimSpace(option.Label) == "" || strings.TrimSpace(option.Description) == "" || labels[option.Label] {
				return model.ToolOutput{}, errors.New("question options require unique labels and descriptions")
			}
			labels[option.Label] = true
		}
	}
	if t.Ask == nil {
		return model.ToolOutput{}, errors.New("AskUserQuestion is unavailable on this host")
	}
	if err := ctx.Err(); err != nil {
		return model.ToolOutput{}, err
	}
	collected, err := t.Ask(ctx, input.Questions)
	if err != nil {
		return model.ToolOutput{}, err
	}
	if err := contracts.ValidateQuestionAnswers(input.Questions, collected); err != nil {
		return model.ToolOutput{}, err
	}
	answers := map[string]string{}
	for i, q := range input.Questions {
		answer := collected[i]
		switch {
		case strings.TrimSpace(answer.Text) != "":
			answers[q.Question] = strings.TrimSpace(answer.Text)
		case q.MultiSelect:
			encoded, err := json.Marshal(answer.Options)
			if err != nil {
				return model.ToolOutput{}, err
			}
			answers[q.Question] = string(encoded)
		default:
			answers[q.Question] = answer.Options[0]
		}
	}
	data := struct {
		Questions []contracts.QuestionPage `json:"questions"`
		Answers   map[string]string        `json:"answers"`
	}{input.Questions, answers}
	output, err := json.Marshal(data)
	return model.ToolOutput{Text: string(output), Data: data}, err
}
