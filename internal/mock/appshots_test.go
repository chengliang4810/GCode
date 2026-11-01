package mock

import (
	"context"
	"reflect"
	"testing"

	"gcode/internal/contracts"
)

func TestAppshotCapabilityMutationsArePlatformScoped(t *testing.T) {
	for _, platform := range []string{"darwin", "windows"} {
		t.Run(platform, func(t *testing.T) {
			s := newTestServer(t, Options{Platform: platform})
			ctx := context.Background()
			before, _ := s.Snapshot(ctx)
			if before.AppshotCapabilities.Platform != platform {
				t.Fatal("platform capability provenance missing")
			}
			if err := s.Execute(ctx, contracts.Command{Kind: "appshots.permissions.check"}); err != nil {
				t.Fatal(err)
			}
			checked, _ := s.Snapshot(ctx)
			if !reflect.DeepEqual(before.AppshotCapabilities, checked.AppshotCapabilities) {
				t.Fatal("permission check granted permission")
			}
			err := s.Execute(ctx, contracts.Command{Kind: "appshots.permission", ID: "windowCapture"})
			after, _ := s.Snapshot(ctx)
			if platform == "darwin" {
				if err != nil || after.AppshotCapabilities.WindowCapture != "ready" || after.AppshotCapabilities.ApplicationText != "permissionRequired" {
					t.Fatal("permission request did not update only the selected capture capability")
				}
				if err = s.Execute(ctx, contracts.Command{Kind: "appshots.permission", ID: "applicationText"}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil || after.AppshotCapabilities.WindowCapture != "unavailable" {
					t.Fatal("unsupported platform acquired capture capability")
				}
			}
		})
	}
}

func TestProductDefaultsFollowReference128(t *testing.T) {
	v := DefaultSettings()
	if v.Appearance.Mode != "system" || v.Appearance.ThemeID != "" || v.General.StopWithEscape || v.Files.Autosave || !v.Appshots.Sound || v.Appshots.Destination != "automatic" || !v.Notifications.Sound {
		t.Fatal("product defaults drifted from the pinned source")
	}
}

func TestTerminalCreateUsesProjectDirectoryAndDeletionSettlesRuns(t *testing.T) {
	s := newTestServer(t, Options{})
	ctx := context.Background()
	if err := s.Execute(ctx, contracts.Command{Kind: "terminal.create", ProjectID: "fieldnotes"}); err != nil {
		t.Fatal(err)
	}
	v, _ := s.Snapshot(ctx)
	if v.Terminals[len(v.Terminals)-1].CWD != v.Projects[0].Path {
		t.Fatal("new terminal used a different project's working directory")
	}
	if err := s.Execute(ctx, contracts.Command{Kind: "terminal.create", ProjectID: "missing"}); err == nil {
		t.Fatal("terminal created for an unknown project")
	}
	run, err := s.Send(ctx, contracts.SendRequest{SessionID: "chat-0", Text: "Review this project"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Execute(ctx, contracts.Command{Kind: "session.delete", ID: "chat-0"}); err != nil {
		t.Fatal(err)
	}
	deleted, _ := s.Snapshot(ctx)
	for _, r := range deleted.Runs {
		if r.ID == run.ID && (r.Status != "interrupted" || r.FinishedAt == nil) {
			t.Fatal("deleted session retained an active run")
		}
	}
	if s.advanceRun("chat-0", run.ID, "late", true) {
		t.Fatal("deleted run accepted late output")
	}
}
