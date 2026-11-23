package workflow

import "gcode/internal/contracts"

// The bounded artifact projections. A run card shows at most three links plus
// a static count; a completion card shows at most six index rows, which
// becomes five rows plus a count when it would overflow.
const (
	CardArtifactLimit      = 3
	CompletionIndexLimit   = 6
	CompletionOverflowRows = 5
)

// ArtifactSlice is the bounded projection of a run's artifacts.
type ArtifactSlice struct {
	Shown    []contracts.WorkflowArtifact
	Overflow int
}

// RunArtifacts returns every latest delivery in publication order.
func RunArtifacts(artifacts []contracts.WorkflowArtifact) []contracts.WorkflowArtifact {
	return orderArtifacts(artifacts)
}

// CardArtifacts projects a run's artifacts for a card: ordered, latest version
// only, at most CardArtifactLimit.
func CardArtifacts(artifacts []contracts.WorkflowArtifact) ArtifactSlice {
	return boundArtifacts(orderArtifacts(artifacts), CardArtifactLimit)
}

// CompletionArtifacts projects already ordered artifacts for a completion card.
func CompletionArtifacts(artifacts []contracts.WorkflowArtifact) ArtifactSlice {
	ordered := orderArtifacts(artifacts)
	if len(ordered) <= CompletionIndexLimit {
		return ArtifactSlice{Shown: ordered}
	}
	return boundArtifacts(ordered, CompletionOverflowRows)
}

// PrimaryArtifact picks the artifact a preview shows. An explicit primary wins;
// a single delivery is the primary on its own.
func PrimaryArtifact(artifacts []contracts.WorkflowArtifact) (contracts.WorkflowArtifact, bool) {
	ordered := orderArtifacts(artifacts)
	for _, artifact := range ordered {
		if artifact.Primary {
			return artifact, true
		}
	}
	if len(ordered) == 1 {
		return ordered[0], true
	}
	return contracts.WorkflowArtifact{}, false
}

func boundArtifacts(ordered []contracts.WorkflowArtifact, limit int) ArtifactSlice {
	if len(ordered) <= limit {
		return ArtifactSlice{Shown: ordered}
	}
	return ArtifactSlice{Shown: ordered[:limit], Overflow: len(ordered) - limit}
}

// orderArtifacts keeps publication order stable and leaves only the latest
// version of each name.
func orderArtifacts(artifacts []contracts.WorkflowArtifact) []contracts.WorkflowArtifact {
	if len(artifacts) == 0 {
		return nil
	}
	ordered := make([]contracts.WorkflowArtifact, 0, len(artifacts))
	seen := make(map[string]int, len(artifacts))
	for _, artifact := range artifacts {
		if index, ok := seen[artifact.Name]; ok {
			if artifact.Version >= ordered[index].Version {
				ordered[index] = artifact
			}
			continue
		}
		seen[artifact.Name] = len(ordered)
		ordered = append(ordered, artifact)
	}
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0; j-- {
			left, right := ordered[j-1].Order, ordered[j].Order
			if left <= right {
				break
			}
			ordered[j-1], ordered[j] = ordered[j], ordered[j-1]
		}
	}
	return ordered
}
