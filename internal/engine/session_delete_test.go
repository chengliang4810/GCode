package engine

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
)

func TestSessionDeleteFailureKeepsTrajectoryUntilIndexCommit(t *testing.T) {
	core := openStorageTestCore(t, t.TempDir())
	defer core.Close()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create"}); err != nil {
		t.Fatal(err)
	}
	state, err := core.Snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	session := state.Sessions[0]
	if err := core.writeTrajectory(session, contracts.ModelInvocation{
		SessionID: session.ID, Invocation: model.Invocation{ID: "delete", Attempt: 1, StartedAt: time.Unix(1, 0), System: "retained diagnostic"},
	}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(core.storage.Directory(), "sessions", session.ID, "trajectory", "call-0000000001000000000-delete-001.json")
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(core.storage.ConfigDirectory(), "engine.json")
	if err := os.Rename(index, index+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(index, 0700); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.delete", ID: session.ID}); err == nil {
		t.Fatal("delete succeeded without index commit")
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatalf("failed delete changed trajectory: %q, %v", after, err)
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(index+".saved", index); err != nil {
		t.Fatal(err)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.delete", ID: session.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("committed delete retained owned trajectory: %v", err)
	}
}
