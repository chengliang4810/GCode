package contracts

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestModelOptionChoiceLabelsAreOptionalMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		labels        bool
	}{
		{"legacy ids", `{"id":"contextWindow","label":"Context window","choices":["standard","1m"],"defaultChoice":"standard"}`, false},
		{"separate labels", `{"id":"contextWindow","label":"Context window","choices":["standard","1m"],"choiceLabels":{"standard":"Standard","1m":"1M tokens"},"defaultChoice":"standard"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var option ModelOption
			if err := json.Unmarshal([]byte(tc.payload), &option); err != nil {
				t.Fatal(err)
			}
			if option.DefaultChoice != "standard" || len(option.Choices) != 2 || option.Choices[1] != "1m" {
				t.Fatal("display metadata replaced the option ids")
			}
			if tc.labels && option.ChoiceLabels["1m"] != "1M tokens" || !tc.labels && option.ChoiceLabels != nil {
				t.Fatal("optional choice labels did not decode")
			}
			encoded, err := json.Marshal(option)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte(`"choiceLabels"`)) != tc.labels {
				t.Fatalf("optional labels were not preserved or omitted: %s", encoded)
			}
		})
	}
}
