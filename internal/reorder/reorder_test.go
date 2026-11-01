package reorder

import (
	"slices"
	"testing"
)

func TestMoveRelativeOwnsResultAndRejectsMissingItems(t *testing.T) {
	for _, tc := range []struct {
		name, source, target string
		after                bool
		want                 []string
		wantErr              bool
	}{
		{name: "up before", source: "c", target: "a", want: []string{"c", "a", "b"}},
		{name: "up after", source: "c", target: "a", after: true, want: []string{"a", "c", "b"}},
		{name: "down before", source: "a", target: "c", want: []string{"b", "a", "c"}},
		{name: "down after", source: "a", target: "c", after: true, want: []string{"b", "c", "a"}},
		{name: "same item", source: "b", target: "b", after: true, want: []string{"a", "b", "c"}},
		{name: "same adjacent position", source: "a", target: "b", want: []string{"a", "b", "c"}},
		{name: "missing source", source: "missing", target: "a", wantErr: true},
		{name: "missing target", source: "a", target: "missing", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []string{"a", "b", "c"}
			result, err := Move(input, tc.source, tc.target, tc.after, func(id string) string { return id })
			if (err != nil) != tc.wantErr {
				t.Fatalf("Move() error = %v", err)
			}
			if !tc.wantErr && !slices.Equal(result, tc.want) {
				t.Fatalf("order = %v, want %v", result, tc.want)
			}
			if len(result) > 0 {
				result[0] = "changed"
			}
			if !slices.Equal(input, []string{"a", "b", "c"}) {
				t.Fatal("move changed caller's slice")
			}
		})
	}
}
