package execution

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestOwnedProcessControlsRejectOtherSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test command uses a POSIX shell")
	}
	root := t.TempDir()
	manager := NewProcessManager(ProcessManagerOptions{})
	request := Request{
		Origin: OriginInteractiveCommand, Mode: ModeCaptured,
		Command:          Command{Name: "/bin/sh", Args: []string{"-c", "sleep 30"}},
		WorkingDirectory: root, WorkspaceRoots: []string{root},
		Approval: ApprovalContext{PolicyVersion: PolicyVersion},
	}
	started, err := manager.StartOwned(context.Background(), "session-a", ProcessStart{
		Prepared: PreparedCommand{Command: exec.Command("/bin/sh", "-c", "sleep 30")}, Request: request,
	}, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Stop(started.ProcessID)
	if _, _, err := manager.SnapshotOwned("session-b", started.ProcessID); !errors.Is(err, ErrProcessSessionMismatch) {
		t.Fatalf("snapshot owner check = %v", err)
	}
	if _, err := manager.ContinueOwned(context.Background(), "session-b", ProcessContinue{ProcessID: started.ProcessID, Interrupt: true}); !errors.Is(err, ErrProcessSessionMismatch) {
		t.Fatalf("continue owner check = %v", err)
	}
	if err := manager.WriteInputOwned("session-b", started.ProcessID, []byte("x")); !errors.Is(err, ErrProcessSessionMismatch) {
		t.Fatalf("input owner check = %v", err)
	}
	if _, err := manager.StopOwned("session-b", started.ProcessID); !errors.Is(err, ErrProcessSessionMismatch) {
		t.Fatalf("stop owner check = %v", err)
	}
	if _, err := manager.StopOwned("session-a", started.ProcessID); err != nil {
		t.Fatal(err)
	}
}
