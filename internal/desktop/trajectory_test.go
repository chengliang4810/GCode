package desktop

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"github.com/egoist/mygo/ui"
)

type trajectoryFixtureService struct {
	contracts.Service
	data contracts.ModelTrajectory
	err  error
}

func (s *trajectoryFixtureService) ReadModelTrajectory(_ context.Context, id string) (contracts.ModelTrajectory, error) {
	data := s.data
	data.SessionID = id
	return data, s.err
}

func TestConversationTitleTrajectoryMenuAndWorkspace(t *testing.T) {
	a := testApp(t, "default")
	now := time.Now()
	fixture := &trajectoryFixtureService{Service: a.service, data: contracts.ModelTrajectory{Records: []contracts.ModelInvocation{
		{Source: "main", Invocation: model.Invocation{ID: "first", Attempt: 1, ModelID: "test-model", StartedAt: now,
			CompletedAt: &now, Messages: []model.Message{{Role: "user", Text: "FIRST_PROMPT"}},
			Response: &model.Result{Message: model.Message{Role: "assistant", Text: "FIRST_RESULT"}, Usage: model.Usage{Known: true, Input: 20, Output: 5}}}},
		{Source: "main", Invocation: model.Invocation{ID: "second", Attempt: 1, ModelID: "test-model", StartedAt: now,
			CompletedAt: &now, Messages: []model.Message{{Role: "user", Text: "SECOND_PROMPT"}},
			Response: &model.Result{Message: model.Message{Role: "assistant", Text: "SECOND_RESULT"}}}},
	}}}
	a.service = fixture
	a.setSelectedSession(a.state.Sessions[0].ID, false)
	sid := a.sessionID
	a.sidebar = false
	tt := ui.NewTester(a.View, 1700, 1100)
	if err := tt.RightClick(a.state.Sessions[0].Title); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(a.t("Call trajectory")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Advance(300 * time.Millisecond)
	if a.sessionID != sid || !tt.HasText("SECOND_RESULT") || !tt.HasText("Call trajectory:") {
		t.Fatalf("right-click trajectory did not open the conversation-bound workspace tab: session=%s right=%v texts=%v", a.sessionID, a.right, tt.Texts())
	}
	if err := tt.Click("Expand all"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("FIRST_RESULT") {
		t.Fatal("expand all omitted the earlier call")
	}
	if err := tt.Click("Collapse all"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("SECOND_RESULT") {
		t.Fatal("collapse all did not collapse the default-open last call")
	}
	state := a.trajectoryViews[sid]
	state.search = "FIRST_RESULT"
	tt.Frame()
	if !tt.HasText("FIRST_RESULT") || tt.HasText("SECOND_RESULT") {
		t.Fatal("search did not reveal only the matching call")
	}
	if err := tt.Click("Copy call"); err != nil {
		t.Fatal(err)
	}
	if text := tt.Clipboard(); !strings.Contains(text, "FIRST_RESULT") {
		t.Fatal("copy call lost the complete structured record")
	}
	fixture.err = errors.New("fixture read failure")
	a.refreshModelTrajectory(sid)
	tt.Frame()
	if !tt.HasText("fixture read failure") || !tt.HasText("FIRST_RESULT") {
		t.Fatal("refresh error discarded the previous readable trajectory")
	}
	if err := tt.Click("Close tab Call trajectory: " + a.state.Sessions[0].Title); err != nil {
		t.Fatal(err)
	}
	if a.sessionID != sid || tt.HasText("FIRST_RESULT") {
		t.Fatal("closing the trajectory changed the central conversation or left its tab open")
	}
	fixture.err, fixture.data.Records = nil, nil
	a.trajectoryViews[sid].search = ""
	a.setPanel(0, true)
	tt.Advance(300 * time.Millisecond)
	bounds, ok := tt.Find(a.state.Sessions[0].Title)
	if !ok || bounds.X >= a.visibleSidebarWidth {
		t.Fatal("sidebar conversation title is not available")
	}
	tt.RightClickAt(bounds.X+bounds.W/2, bounds.Y+bounds.H/2)
	if err := tt.ChooseMenuItem("Call trajectory"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if !tt.HasText("No recorded model calls") {
		t.Fatal("sidebar entry did not open the empty historical trajectory")
	}
}
