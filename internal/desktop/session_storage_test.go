package desktop

import (
	"context"
	"errors"
	"testing"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

type sessionStorageNoticeService struct {
	contracts.Service
	snapshot contracts.Snapshot
	retried  []string
	err      error
}

func (s *sessionStorageNoticeService) Snapshot(context.Context) (contracts.Snapshot, error) {
	return contracts.CloneSnapshot(s.snapshot), nil
}

func (s *sessionStorageNoticeService) Execute(_ context.Context, command contracts.Command) error {
	if command.Kind != "session.storage.retry" {
		return errors.New("unexpected test command: " + command.Kind)
	}
	s.retried = append(s.retried, command.SessionID)
	return s.err
}

func TestSessionStorageErrorVisibleInBothConversationPanes(t *testing.T) {
	for _, pane := range []string{"main", "side"} {
		for _, theme := range []string{"rcode-light", "rcode-dark"} {
			t.Run(pane+"/"+theme, func(t *testing.T) {
				app := testApp(t, "transcript")
				app.selectSession("chat-0")
				session := app.currentSession()
				session.StorageError = "original data root is offline"
				for i := range app.state.Sessions {
					if app.state.Sessions[i].ID == session.ID {
						app.state.Sessions[i] = session
					}
				}
				app.state.Messages[session.ID] = nil
				service := &sessionStorageNoticeService{
					Service: app.service, snapshot: contracts.CloneSnapshot(app.state),
					err: errors.New("original data root is still offline"),
				}
				app.service = service
				app.palette = design.Palette(theme, theme == "rcode-dark")
				tt := ui.NewTester(func(c *ui.Context) {
					c.SetTheme(app.palette.UI())
					if pane == "main" {
						app.chatView(c)
					} else {
						app.sideTranscript(c, session.ID)
					}
				}, 900, 600)
				if !tt.HasText("Conversation data is unavailable") || !tt.HasText(session.StorageError) {
					t.Fatalf("offline history looks empty instead of unavailable: %v", tt.Texts())
				}
				if err := tt.Click("Retry loading conversation"); err != nil {
					t.Fatal(err)
				}
				if len(service.retried) != 1 || service.retried[0] != session.ID {
					t.Fatalf("retry targeted the wrong conversation: %v", service.retried)
				}
				if !tt.HasText("Conversation data is unavailable") {
					t.Fatal("failed retry hid the storage error")
				}
				for i := range service.snapshot.Sessions {
					if service.snapshot.Sessions[i].ID == session.ID {
						service.snapshot.Sessions[i].StorageError = ""
					}
				}
				service.snapshot.Revision++
				app.apply(service.snapshot)
				tt.Frame()
				if tt.HasText("Conversation data is unavailable") {
					t.Fatal("the recovered authoritative snapshot retained the storage error")
				}
			})
		}
	}
}
