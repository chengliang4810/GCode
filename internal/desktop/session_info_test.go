package desktop

import (
	"fmt"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/design"
	"github.com/egoist/mygo/ui"
)

func TestSessionInfoOnlyAggregatesAgents(t *testing.T) {
	for _, theme := range []string{"light", "dark"} {
		for _, width := range []float32{900, 1600} {
			t.Run(fmt.Sprintf("%s/%g", theme, width), func(t *testing.T) {
				a := testApp(t, "default")
				a.palette = design.Palette("rcode-"+theme, theme == "dark")
				a.selectSession("chat-0")
				a.draft = "Keep parent draft"
				a.expanded["session-info-user"], a.expanded["session-info"] = true, true
				a.state.Sessions = append(a.state.Sessions,
					contracts.Session{ID: "nested-agent", Title: "Nested finished agent", ParentSessionID: "finished-agent", SubagentType: "Explore", SideChat: true, Archived: true, LastOutcome: "completed"},
					contracts.Session{ID: "active-agent", Title: "Active agent", ParentSessionID: "chat-0", SubagentType: "Explore", SideChat: true, Status: contracts.Working},
					contracts.Session{ID: "finished-agent", Title: "Finished agent", ParentSessionID: "chat-0", SubagentType: "Explore", SideChat: true, LastOutcome: "completed"},
					contracts.Session{ID: "ended-agent", Title: "Ended agent", ParentSessionID: "chat-0", SubagentType: "Explore", SideChat: true, Archived: true, LastOutcome: "cancelled"},
					contracts.Session{ID: "ordinary-side", Title: "Ordinary side chat", ParentSessionID: "chat-0", SideChat: true},
					contracts.Session{ID: "copied-chat", Title: "Copied conversation", ParentSessionID: "chat-0"},
					contracts.Session{ID: "unrelated-agent", Title: "Unrelated agent", ParentSessionID: "chat-1", SubagentType: "Explore"},
					contracts.Session{ID: "side-agent", Title: "Side chat agent", ParentSessionID: "ordinary-side", SubagentType: "Explore"},
				)
				start := a.now().Add(-12900 * time.Millisecond)
				finish := start.Add(8900 * time.Millisecond)
				a.state.Runs = append(a.state.Runs,
					contracts.Run{ID: "active-run", SessionID: "active-agent", Status: "running", StartedAt: start},
					contracts.Run{ID: "finished-run", SessionID: "finished-agent", Status: "completed", StartedAt: start, FinishedAt: &finish},
				)
				publishMetricsFixture(a)
				a.workspace.Update(a.state)
				tt := ui.NewTester(func(c *ui.Context) { c.SetTheme(a.palette.UI()); a.chatWidth = width; a.sessionInfo(c) }, int(width), 900)
				tt.Frame()
				if !tt.HasText("Agents") {
					t.Fatal("missing agent heading")
				}
				if _, ok := tt.Find("Active agent"); !ok {
					t.Fatal("running agent is hidden")
				}
				for _, label := range []string{"Finished agent", "Ended agent", "Nested finished agent"} {
					if _, ok := tt.Find(label); ok {
						t.Fatalf("non-running agent is flat: %q", label)
					}
				}
				if !tt.HasText("3") {
					t.Fatal("ended group count is missing")
				}
				writeModelSurfaceEvidence(t, fmt.Sprintf("session-agents-collapsed-%s-%g", theme, width), tt, map[string]any{"scope": "non-running agents collapsed"})
				if err := tt.Click("Ended"); err != nil {
					t.Fatal(err)
				}
				for _, label := range []string{"Active agent", "Finished agent", "Ended agent", "Nested finished agent"} {
					if _, ok := tt.Find(label); !ok {
						t.Fatalf("missing %q", label)
					}
				}
				for _, label := range []string{"Ordinary side chat", "Copied conversation", "Unrelated agent", "Side chat agent", "Fork chat", "SIDE CHATS"} {
					if _, ok := tt.Find(label); ok {
						t.Fatalf("unexpected %q", label)
					}
				}
				for _, text := range []string{"12s", "8s", "—"} {
					if !tt.HasText(text) {
						t.Fatalf("missing agent metadata %q", text)
					}
				}
				now := a.now()
				a.now = func() time.Time { return now.Add(3 * time.Second) }
				tt.Advance(time.Second)
				if !tt.HasText("15s") || !tt.HasText("8s") {
					t.Fatal("running time did not update or completed time changed")
				}
				writeModelSurfaceEvidence(t, fmt.Sprintf("session-agents-%s-%g", theme, width), tt, map[string]any{"scope": "agent state and elapsed time"})
				if err := tt.Click("Ended"); err != nil {
					t.Fatal(err)
				}
				if _, ok := tt.Find("Finished agent"); ok {
					t.Fatal("group did not collapse")
				}
				for i := range a.state.Sessions {
					if a.state.Sessions[i].ID == "active-agent" {
						a.state.Sessions[i].Status = contracts.Idle
						a.state.Sessions[i].LastOutcome = "completed"
					}
				}
				a.state.Revision++
				tt.Frame()
				if _, ok := tt.Find("Active agent"); ok {
					t.Fatal("finished agent remained outside collapsed group")
				}
				if !tt.HasText("4") {
					t.Fatal("group count did not update after completion")
				}
				if err := tt.Click("Ended"); err != nil {
					t.Fatal(err)
				}
				if err := tt.Click("Ended agent"); err != nil {
					t.Fatal(err)
				}
				tt.Advance(300 * time.Millisecond)
				if a.workspace.ActiveKind() != "session" || a.sessionID != "chat-0" || a.draft != "Keep parent draft" {
					t.Fatal("archived agent did not open independently")
				}
			})
		}
	}
}

func TestSessionAgentElapsedDigitsStayStable(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprintf("dark=%v", dark), func(t *testing.T) {
			a := testApp(t, "default")
			a.palette = design.Palette("rcode-dark", dark)
			duration := "1m 11s"
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(a.palette.UI())
				a.controlSessionAgent(c, "Agent", "Running", duration)
			}, 320, 80)
			initial, ok := tt.Find(duration)
			if !ok {
				t.Fatal("agent elapsed time is missing")
			}
			for _, seconds := range []int{12, 18, 19, 20, 28, 48, 59} {
				duration = fmt.Sprintf("1m %02ds", seconds)
				tt.Frame()
				if bounds, ok := tt.Find(duration); !ok || bounds != initial {
					t.Fatalf("agent duration moved at %s: %+v, want %+v", duration, bounds, initial)
				}
			}
		})
	}
}

func TestSessionAgentStatus(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session contracts.Session
		want    string
	}{
		{"running overrides previous outcome", contracts.Session{Status: contracts.Working, LastOutcome: "completed"}, "Running"},
		{"awaiting input", contracts.Session{Status: contracts.AwaitingInput}, "Awaiting input"},
		{"error", contracts.Session{Status: contracts.Errored}, "Failed"},
		{"completed archived", contracts.Session{LastOutcome: "completed", Archived: true}, "Completed"},
		{"failed", contracts.Session{LastOutcome: "failed"}, "Failed"},
		{"interrupted", contracts.Session{LastOutcome: "interrupted"}, "Stopped"},
		{"cancelled", contracts.Session{LastOutcome: "cancelled", Archived: true}, "Stopped"},
		{"ended", contracts.Session{Archived: true}, "Ended"},
		{"pending", contracts.Session{Status: contracts.Idle}, "Pending"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionAgentStatus(tc.session); got != tc.want {
				t.Fatalf("status=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestSessionAgentDurationUsesOwnRuns(t *testing.T) {
	a := testApp(t, "default")
	start := a.now().Add(-time.Minute)
	firstEnd := start.Add(10 * time.Second)
	a.state.Runs = []contracts.Run{
		{ID: "first", SessionID: "agent", StartedAt: start, FinishedAt: &firstEnd, Status: "completed"},
		{ID: "second", SessionID: "agent", StartedAt: start.Add(50 * time.Second), Status: "running"},
		{ID: "nested", SessionID: "nested-agent", StartedAt: start, Status: "running"},
	}
	publishMetricsFixture(a)
	if elapsed, known := a.sessionAgentDuration("agent"); !known || elapsed != 20*time.Second {
		t.Fatalf("duration includes idle gaps or another agent: %v, %v", elapsed, known)
	}
	publishMetricsFixture(a)
	if elapsed, known := a.sessionAgentDuration("missing"); known || elapsed != 0 {
		t.Fatalf("missing records reported a duration: %v, %v", elapsed, known)
	}
	a.state.Runs = append(a.state.Runs, contracts.Run{ID: "instant", SessionID: "instant-agent", StartedAt: start, FinishedAt: &start, Status: "completed"})
	publishMetricsFixture(a)
	if elapsed, known := a.sessionAgentDuration("instant-agent"); !known || elapsed != 0 {
		t.Fatalf("zero-duration execution treated as unknown: %v, %v", elapsed, known)
	}
}

func TestSessionInfoHidesEmptyAgentCard(t *testing.T) {
	a := testApp(t, "default")
	a.selectSession("chat-0")
	a.expanded["session-info-user"], a.expanded["session-info"] = true, true
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(a.palette.UI())
		a.chatWidth = 900
		a.sessionInfo(c)
	}, 900, 700)
	tt.Frame()
	if tt.HasText("Agents") {
		t.Fatal("empty agent card is visible")
	}
	if _, ok := tt.Find("Branch changes"); !ok {
		t.Fatal("session details disappeared with empty agent card")
	}
}
