package mock

import (
	"slices"

	"gcode/internal/contracts"
)

// cloneSnapshot delegates to the contract's deep copy. Scenarios and component
// tests share the real engine's ownership rule: every subscriber owns its
// snapshot, so a UI that edits one projection cannot mutate the backend state or
// another subscriber.
func cloneSnapshot(v contracts.Snapshot) contracts.Snapshot {
	return contracts.CloneSnapshot(v)
}

func cloneBlocks(src []contracts.Block) []contracts.Block {
	out := slices.Clone(src)
	for i := range out {
		b := &out[i]
		if b.Annotation != nil {
			v := b.Annotation.Bounded()
			b.Annotation = &v
		}
		if b.Tool != nil {
			v := *b.Tool
			if v.ExitCode != nil {
				code := *v.ExitCode
				v.ExitCode = &code
			}
			if v.Diff != nil {
				change := *v.Diff
				change.Hunks = slices.Clone(change.Hunks)
				for hi := range change.Hunks {
					change.Hunks[hi].Lines = slices.Clone(change.Hunks[hi].Lines)
				}
				v.Diff = &change
			}
			if v.FinishedAt != nil {
				t := *v.FinishedAt
				v.FinishedAt = &t
			}
			b.Tool = &v
		}
		if b.Question != nil {
			v := *b.Question
			v.Pages = slices.Clone(v.Pages)
			for i := range v.Pages {
				v.Pages[i].Options = slices.Clone(v.Pages[i].Options)
			}
			v.Answers = contracts.CloneQuestionAnswers(v.Answers)
			b.Question = &v
		}
		if b.Permission != nil {
			v := *b.Permission
			b.Permission = &v
		}
		b.Todos = slices.Clone(b.Todos)
		if b.Subagent != nil {
			v := *b.Subagent
			b.Subagent = &v
		}
		if b.Attachment != nil {
			v := *b.Attachment
			b.Attachment = &v
		}
		if b.Artifact != nil {
			v := *b.Artifact
			b.Artifact = &v
		}
	}
	return out
}
