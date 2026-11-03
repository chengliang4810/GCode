package contracts

import "testing"

func TestQuestionAnswersValidateEntireRequest(t *testing.T) {
	pages := []QuestionPage{
		{Question: "Mode", Options: []QuestionOption{{Label: "A"}, {Label: "B"}}},
		{Question: "Features", MultiSelect: true, Options: []QuestionOption{{Label: "X"}, {Label: "Y"}}},
	}
	for _, test := range []struct {
		name    string
		answers []QuestionAnswer
		valid   bool
	}{
		{"selections", []QuestionAnswer{{Options: []string{"A"}}, {Options: []string{"X", "Y"}}}, true},
		{"custom", []QuestionAnswer{{Text: "another mode"}, {Text: "different features"}}, true},
		{"missing page", []QuestionAnswer{{Options: []string{"A"}}}, false},
		{"empty", []QuestionAnswer{{}, {}}, false},
		{"unknown choice", []QuestionAnswer{{Options: []string{"unknown"}}, {Options: []string{"X"}}}, false},
		{"duplicate choice", []QuestionAnswer{{Options: []string{"A"}}, {Options: []string{"X", "X"}}}, false},
		{"single picks two", []QuestionAnswer{{Options: []string{"A", "B"}}, {Options: []string{"X"}}}, false},
		{"mixed text and choice", []QuestionAnswer{{Text: "custom", Options: []string{"A"}}, {Options: []string{"X"}}}, false},
		{"blank text", []QuestionAnswer{{Text: " \n "}, {Options: []string{"X"}}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateQuestionAnswers(pages, test.answers); (err == nil) != test.valid {
				t.Fatalf("valid=%t: %v", test.valid, err)
			}
		})
	}
}

func TestQuestionSnapshotAndCommandOwnNestedValues(t *testing.T) {
	question := &Question{ID: "request", Pages: []QuestionPage{{Question: "Mode", Options: []QuestionOption{{Label: "A", Description: "First"}}}},
		Answers: []QuestionAnswer{{Options: []string{"A"}}}}
	snapshot := Snapshot{Messages: map[string][]Message{"session": {{Blocks: []Block{{Question: question}}}}}}
	copy := CloneSnapshot(snapshot)
	copyQuestion := copy.Messages["session"][0].Blocks[0].Question
	copyQuestion.Pages[0].Options[0].Label = "changed"
	copyQuestion.Answers[0].Options[0] = "changed"
	if question.Pages[0].Options[0].Label != "A" || question.Answers[0].Options[0] != "A" {
		t.Fatal("snapshot retained mutable question storage")
	}
	command := Command{Kind: "request.answer", ID: "request", QuestionAnswers: question.Answers}
	frozen := command.Clone()
	command.QuestionAnswers[0].Options[0] = "changed"
	if frozen.QuestionAnswers[0].Options[0] != "A" {
		t.Fatal("queued answer retained the UI's mutable selection")
	}
}
