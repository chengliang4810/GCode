package engine

import (
	"context"
	"net/http"
	"testing"
	"time"

	"gcode/internal/contracts"
	"gcode/internal/model"
	"gcode/internal/storage"
)

func TestAutomationSchedule(t *testing.T) {
	now := time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		job     contracts.Automation
		want    time.Time
		invalid bool
	}{
		{name: "weekday timezone", job: contracts.Automation{Mode: "cron", Cron: "0 9 * * 1-5", Timezone: "Asia/Shanghai", Enabled: true}, want: now.Add(time.Hour)},
		{name: "completion interval", job: contracts.Automation{Mode: "interval", IntervalSeconds: 90, Enabled: true}, want: now.Add(90 * time.Second)},
		{name: "manual", job: contracts.Automation{Mode: "manual", Enabled: true}},
		{name: "paused", job: contracts.Automation{Mode: "cron"}},
		{name: "bad cron", job: contracts.Automation{Mode: "cron", Cron: "oops", Timezone: "UTC", Enabled: true}, invalid: true},
		{name: "impossible date", job: contracts.Automation{Mode: "cron", Cron: "0 9 31 2 *", Timezone: "UTC", Enabled: true}, invalid: true},
		{name: "invalid timezone", job: contracts.Automation{Mode: "cron", Cron: "0 9 * * *", Timezone: "No/Zone", Enabled: true}, invalid: true},
		{name: "short interval", job: contracts.Automation{Mode: "interval", IntervalSeconds: 1, Enabled: true}, invalid: true},
		{name: "overflow interval", job: contracts.Automation{Mode: "interval", IntervalSeconds: 1 << 62, Enabled: true}, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nextAutomation(tc.job, now)
			if (err != nil) != tc.invalid || !tc.invalid && !got.Equal(tc.want) {
				t.Fatalf("next=%v err=%v want=%v", got, err, tc.want)
			}
		})
	}
}

func createTestAutomation(t *testing.T, core *Engine, mode string) contracts.Automation {
	t.Helper()
	snapshot, _ := core.Snapshot(t.Context())
	a := contracts.Automation{Name: "Scheduled review", Prompt: "Reply with Done", ModelID: "goal/model", ProjectID: snapshot.Projects[0].ID, Mode: mode, Cron: "0 9 * * *", Timezone: "Asia/Shanghai", IntervalSeconds: 60, Enabled: true, Continuous: true}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.save", Automation: &a}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = core.Snapshot(t.Context())
	return snapshot.Automations[len(snapshot.Automations)-1]
}

func TestAutomationExecutesRealCoreAndContinuesSession(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	job := createTestAutomation(t, core, "interval")
	for n := 0; n < 2; n++ {
		if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.run", ID: job.ID}); err != nil {
			t.Fatal(err)
		}
		waitIdle(t, core)
		core.automationTick(time.Now())
	}
	state, _ := core.Snapshot(t.Context())
	if len(state.AutomationRuns) != 2 {
		t.Fatalf("runs=%+v", state.AutomationRuns)
	}
	for _, run := range state.AutomationRuns {
		if run.Status != "completed" || run.FinishedAt.IsZero() {
			t.Fatalf("run=%+v", run)
		}
	}
	if state.AutomationRuns[0].SessionID != state.AutomationRuns[1].SessionID {
		t.Fatal("continuous automation changed sessions")
	}
	last := state.AutomationRuns[1]
	if !state.Automations[0].NextAt.Equal(last.FinishedAt.Add(time.Minute)) {
		t.Fatal("interval did not start at completion")
	}
	// Paused jobs never fire just because they have a stale due timestamp.
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.toggle", ID: job.ID, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	core.fireDueAutomations(time.Now().Add(24 * time.Hour))
	state, _ = core.Snapshot(t.Context())
	if len(state.AutomationRuns) != 2 {
		t.Fatal("paused job fired")
	}
}

func TestAutomationDueOnceFailureAndNewSession(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	job := createTestAutomation(t, core, "cron")
	core.mu.Lock()
	core.state.Automations[0].Once = true
	core.state.Automations[0].Continuous = false
	core.state.Automations[0].NextAt = time.Now().Add(-time.Minute)
	core.mu.Unlock()
	core.fireDueAutomations(time.Now())
	waitIdle(t, core)
	core.automationTick(time.Now())
	state, _ := core.Snapshot(t.Context())
	if state.Automations[0].Enabled || len(state.AutomationRuns) != 1 {
		t.Fatalf("once was not consumed: %+v", state.Automations)
	}
	// A manual invocation remains available after a once job completes.
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.run", ID: job.ID}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, core)
	core.automationTick(time.Now())
	state, _ = core.Snapshot(t.Context())
	if state.AutomationRuns[0].SessionID == state.AutomationRuns[1].SessionID {
		t.Fatal("new-each-run reused a conversation")
	}
	// Missing models produce a durable failed attempt rather than losing history.
	core.mu.Lock()
	core.state.Automations[0].ModelID = "missing/model"
	core.mu.Unlock()
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.run", ID: job.ID}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(t.Context())
	last := state.AutomationRuns[len(state.AutomationRuns)-1]
	if last.Status != "failed" || last.Error == "" {
		t.Fatalf("failure not recorded: %+v", last)
	}
}

func TestAutomationNoOverlapAndPauseCancels(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	defer close(release)
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
			return
		case <-release:
			goalResponse(w, model.Chat, "Done", nil, true)
		}
	}))
	job := createTestAutomation(t, core, "interval")
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.run", ID: job.ID}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("model request never started")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.run", ID: job.ID}); err == nil {
		t.Fatal("overlapping run accepted")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.toggle", ID: job.ID, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, core)
	core.automationTick(time.Now())
	state, _ := core.Snapshot(t.Context())
	if state.Automations[0].Enabled || len(state.AutomationRuns) != 1 || state.AutomationRuns[0].Status != "interrupted" {
		t.Fatalf("pause did not stop run: %+v", state.AutomationRuns)
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.delete", ID: job.ID}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(t.Context())
	if len(state.Automations) != 0 || len(state.Sessions) == 0 {
		t.Fatal("delete must preserve conversations")
	}
}

func TestAutomationPersistsAndSkipsOfflineSlots(t *testing.T) {
	home := t.TempDir()
	store, err := storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	core, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	core.mu.Lock()
	core.state.Automations = []contracts.Automation{{ID: "offline", Name: "Offline", Mode: "interval", IntervalSeconds: 60, Enabled: true, NextAt: now.Add(-time.Hour)}}
	core.state.AutomationRuns = []contracts.AutomationRun{{ID: "accepted", AutomationID: "offline", Status: "running", StartedAt: now.Add(-time.Hour)}}
	core.mu.Unlock()
	if err = core.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(storage.Options{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := openExecutionFixture(t, Options{Storage: store})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err := reopened.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.AutomationRuns[0].Status != "interrupted" || !state.Automations[0].NextAt.After(now) || len(state.Runs) != 0 {
		t.Fatalf("unexpected recovery: %+v %+v", state.Automations, state.AutomationRuns)
	}
	state.Automations[0].Name = "changed"
	next, _ := reopened.Snapshot(t.Context())
	if next.Automations[0].Name == "changed" {
		t.Fatal("snapshot aliases core")
	}
}

func TestAutomationBindsExistingSessionAndUnbindsExplicitly(t *testing.T) {
	core, _ := goalTestCore(t, model.Chat, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = subagentRequest(t, r)
		goalResponse(w, model.Chat, "Done", nil, true)
	}))
	state, _ := core.Snapshot(t.Context())
	if err := core.Execute(t.Context(), contracts.Command{Kind: "session.create", ProjectID: state.Projects[0].ID, Name: "Existing work"}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(t.Context())
	target := state.Sessions[0].ID
	job := contracts.Automation{Name: "Bound", Prompt: "Continue existing work", ModelID: "goal/model", Mode: "manual", Enabled: true, TargetSessionID: target}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.save", Automation: &job}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(t.Context())
	job = state.Automations[0]
	if job.SessionID != target || !job.Continuous || job.ProjectID != state.Projects[0].ID {
		t.Fatal("binding did not inherit target workspace")
	}
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.run", ID: job.ID}); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, core)
	core.automationTick(time.Now())
	state, _ = core.Snapshot(t.Context())
	if len(state.Sessions) != 1 || !state.Sessions[0].Unread {
		t.Fatal("bound run lost session identity or completion unread state")
	}
	job.TargetSessionID = ""
	if err := core.Execute(t.Context(), contracts.Command{Kind: "automation.save", Automation: &job}); err != nil {
		t.Fatal(err)
	}
	state, _ = core.Snapshot(t.Context())
	if state.Automations[0].SessionID != "" {
		t.Fatal("clearing binding silently reused the previous target")
	}
}
