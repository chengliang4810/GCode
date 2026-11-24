package workflow

import "testing"

func TestParseActorReplyRequiresArtifactContent(t *testing.T) {
	reply := ParseActorReply(ArtifactMarker + "Title only" + ArtifactMarker + "\n" +
		ArtifactMarker + "Report.md\n# Overview\n| A | B |" + ArtifactMarker + "\n" +
		ArtifactMarker + "Summary|One line" + ArtifactMarker)
	if len(reply.Artifacts) != 2 ||
		reply.Artifacts[0] != (ArtifactDraft{Name: "Report.md", Content: "# Overview\n| A | B |"}) ||
		reply.Artifacts[1] != (ArtifactDraft{Name: "Summary", Content: "One line"}) {
		t.Fatalf("unexpected artifacts: %#v", reply.Artifacts)
	}
}
