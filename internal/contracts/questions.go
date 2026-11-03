package contracts

import (
	"errors"
	"slices"
	"strings"
)

func CloneQuestionAnswers(answers []QuestionAnswer) []QuestionAnswer {
	answers = slices.Clone(answers)
	for i := range answers {
		answers[i].Options = slices.Clone(answers[i].Options)
	}
	return answers
}

// ValidateQuestionAnswers checks the entire request before resolving any page.
func ValidateQuestionAnswers(pages []QuestionPage, answers []QuestionAnswer) error {
	if len(pages) == 0 || len(answers) != len(pages) {
		return errors.New("answer every question before submitting")
	}
	for i, answer := range answers {
		if strings.TrimSpace(answer.Text) != "" {
			if len(answer.Options) != 0 {
				return errors.New("an answer must contain either text or selected options")
			}
			continue
		}
		if len(answer.Options) == 0 || (!pages[i].MultiSelect && len(answer.Options) != 1) {
			return errors.New("select an option or enter an answer")
		}
		seen := map[string]bool{}
		for _, selected := range answer.Options {
			if seen[selected] || !slices.ContainsFunc(pages[i].Options, func(option QuestionOption) bool { return option.Label == selected }) {
				return errors.New("answer contains an invalid or repeated option")
			}
			seen[selected] = true
		}
	}
	return nil
}
