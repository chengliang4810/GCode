package desktop

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

type cloudNavigationService struct {
	contracts.Service
	commands       []contracts.Command
	failKind       string
	executeErr     error
	snapshotErr    error
	onExecute      func(context.Context, contracts.Command)
	adjustSnapshot func(*contracts.Snapshot)
}

func (s *cloudNavigationService) Execute(ctx context.Context, command contracts.Command) error {
	if !strings.HasPrefix(command.Kind, "cloud.") {
		return s.Service.Execute(ctx, command)
	}
	s.commands = append(s.commands, command.Clone())
	var err error
	if command.Kind == s.failKind {
		err = s.executeErr
	}
	if s.onExecute != nil {
		s.onExecute(ctx, command)
	}
	if err != nil {
		return err
	}
	return s.Service.Execute(ctx, command)
}

func (s *cloudNavigationService) Snapshot(ctx context.Context) (contracts.Snapshot, error) {
	if s.snapshotErr != nil {
		return contracts.Snapshot{}, s.snapshotErr
	}
	snapshot, err := s.Service.Snapshot(ctx)
	if err == nil && s.adjustSnapshot != nil {
		s.adjustSnapshot(&snapshot)
	}
	return snapshot, err
}

func cloudNavigationTestApp(t *testing.T) (*App, *cloudNavigationService) {
	t.Helper()
	a, _ := settingsTestApp(t)
	// Start with the mock account available and its Cloud instance turned off.
	a.dispatch(contracts.Command{Kind: "cloud.delete"})
	service := &cloudNavigationService{Service: a.service}
	a.service = service
	return a, service
}

func cloudNavigationKinds(t *testing.T, service *cloudNavigationService) []string {
	t.Helper()
	var kinds []string
	for _, command := range service.commands {
		if err := command.Validate(); err != nil {
			t.Fatalf("invalid typed Cloud request: %v", err)
		}
		kinds = append(kinds, command.Kind)
	}
	return kinds
}

func TestCloudSettingsNavigationEnablesOnlyFreshAvailableOff(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *App, *cloudNavigationService)
		want  []string
	}{
		{name: "signed in and off", want: []string{"cloud.refresh", "cloud.enable"}},
		{name: "cached ready but backend off", setup: func(_ *testing.T, a *App, _ *cloudNavigationService) {
			a.state.Integrations.Cloud.State = "ready"
			a.state.Integrations.Cloud.DeviceID = "cloud-1"
		}, want: []string{"cloud.refresh", "cloud.enable"}},
		{name: "cached off but backend enabled", setup: func(t *testing.T, a *App, service *cloudNavigationService) {
			if err := service.Service.Execute(a.ctx, contracts.Command{Kind: "cloud.enable"}); err != nil {
				t.Fatal(err)
			}
		}, want: []string{"cloud.refresh"}},
		{name: "signed out", setup: func(_ *testing.T, a *App, service *cloudNavigationService) {
			service.adjustSnapshot = func(snapshot *contracts.Snapshot) {
				snapshot.User.SignedIn = false
				snapshot.Integrations.Cloud.Available = false
				snapshot.Integrations.Billing.Available = false
			}
			service.adjustSnapshot(&a.state)
		}},
		{name: "unavailable", setup: func(_ *testing.T, _ *App, service *cloudNavigationService) {
			service.adjustSnapshot = func(snapshot *contracts.Snapshot) { snapshot.Integrations.Cloud.Available = false }
		}, want: []string{"cloud.refresh"}},
		{name: "loading", setup: func(_ *testing.T, _ *App, service *cloudNavigationService) {
			service.adjustSnapshot = func(snapshot *contracts.Snapshot) { snapshot.Integrations.Cloud.LoadState = contracts.LoadLoading }
		}, want: []string{"cloud.refresh"}},
		{name: "status error", setup: func(_ *testing.T, _ *App, service *cloudNavigationService) {
			service.adjustSnapshot = func(snapshot *contracts.Snapshot) { snapshot.Integrations.Cloud.LoadState = contracts.LoadError }
		}, want: []string{"cloud.refresh"}},
		{name: "provisioning", setup: func(_ *testing.T, _ *App, service *cloudNavigationService) {
			service.adjustSnapshot = func(snapshot *contracts.Snapshot) { snapshot.Integrations.Cloud.State = "provisioning" }
		}, want: []string{"cloud.refresh"}},
		{name: "deleting", setup: func(_ *testing.T, _ *App, service *cloudNavigationService) {
			service.adjustSnapshot = func(snapshot *contracts.Snapshot) { snapshot.Integrations.Cloud.State = "deleting" }
		}, want: []string{"cloud.refresh"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, service := cloudNavigationTestApp(t)
			if test.setup != nil {
				test.setup(t, a, service)
			}
			a.OpenRoute("settings/cloud")
			tt := ui.NewTester(a.View, 1320, 1000)
			for range 3 {
				a.OpenRoute("settings/Cloud")
				tt.Frame()
			}
			if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Cloud requests = %v, want %v", got, test.want)
			}
			if len(test.want) == 2 && (a.state.Integrations.Cloud.State != "ready" || a.state.Integrations.Cloud.DeviceID != "cloud-1") {
				t.Fatalf("enable did not reach stateful mock: %+v", a.state.Integrations.Cloud)
			}
		})
	}
}

func TestCloudSettingsNavigationFromSectionAndSearch(t *testing.T) {
	for _, search := range []bool{false, true} {
		name := "section"
		if search {
			name = "settings search"
		}
		t.Run(name, func(t *testing.T) {
			a, service := cloudNavigationTestApp(t)
			a.OpenRoute("settings/General")
			tt := ui.NewTester(a.View, 1320, 1000)
			if search {
				if err := tt.Click("Search settings"); err != nil {
					t.Fatal(err)
				}
				tt.Type("Cloud")
			}
			if err := tt.Click("Cloud"); err != nil {
				t.Fatal(err)
			}
			if a.section != "Cloud" || a.state.Integrations.Cloud.State != "ready" {
				t.Fatalf("visible Cloud navigation did not enable: section=%s status=%+v", a.section, a.state.Integrations.Cloud)
			}
			if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, []string{"cloud.refresh", "cloud.enable"}) {
				t.Fatalf("Cloud requests = %v", got)
			}
		})
	}
}

func TestCloudSettingsNavigationFailureWaitsForNewVisit(t *testing.T) {
	for _, phase := range []string{"refresh", "enable", "snapshot"} {
		t.Run(phase, func(t *testing.T) {
			a, service := cloudNavigationTestApp(t)
			failure := errors.New("Engine not connected")
			if phase == "snapshot" {
				service.snapshotErr = failure
			} else {
				service.failKind, service.executeErr = "cloud."+phase, failure
			}
			a.OpenRoute("settings/Cloud")
			initial := len(service.commands)
			tt := ui.NewTester(a.View, 1320, 1000)
			if !tt.HasText(a.cloudPage.error) || !strings.Contains(a.cloudPage.error, failure.Error()) {
				t.Fatalf("failed page request was not visible: %q", a.cloudPage.error)
			}
			for range 4 {
				a.OpenRoute("settings/Cloud")
				tt.Frame()
			}
			if len(service.commands) != initial {
				t.Fatal("a redraw or same-page navigation retried a failed request")
			}
			service.executeErr, service.snapshotErr = nil, nil
			a.OpenRoute("settings/Devices")
			a.OpenRoute("settings/Cloud")
			tt.Frame()
			if len(service.commands) != initial+2 || a.cloudPage.error != "" || a.state.Integrations.Cloud.State != "ready" {
				t.Fatalf("new visit did not recover: requests=%v status=%+v error=%q", cloudNavigationKinds(t, service), a.state.Integrations.Cloud, a.cloudPage.error)
			}
		})
	}
}

func TestCloudSettingsNavigationPendingReadAndActionAreNotDuplicated(t *testing.T) {
	a, service := cloudNavigationTestApp(t)
	service.onExecute = func(_ context.Context, _ contracts.Command) {
		// Run at the request/completion boundary while the page is busy.
		for range 3 {
			a.OpenRoute("settings/Cloud")
			a.syncCloudSettingsPage()
		}
	}
	a.OpenRoute("settings/Cloud")
	if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, []string{"cloud.refresh", "cloud.enable"}) {
		t.Fatalf("busy navigation duplicated a request: %v", got)
	}
}

func TestCloudSettingsNavigationRejectsCancelledVisitFailure(t *testing.T) {
	a, service := cloudNavigationTestApp(t)
	service.failKind, service.executeErr = "cloud.enable", errors.New("old visit failed")
	var oldContext context.Context
	service.onExecute = func(ctx context.Context, command contracts.Command) {
		if command.Kind != "cloud.enable" {
			return
		}
		oldContext = ctx
		service.onExecute, service.executeErr = nil, nil
		a.OpenRoute("settings/Devices")
		a.OpenRoute("settings/Cloud")
	}
	a.OpenRoute("settings/Cloud")
	if oldContext == nil || !errors.Is(oldContext.Err(), context.Canceled) {
		t.Fatal("leaving Cloud did not cancel its pending request")
	}
	if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, []string{"cloud.refresh", "cloud.enable", "cloud.refresh", "cloud.enable"}) {
		t.Fatalf("unexpected visit requests: %v", got)
	}
	if a.cloudPage.error != "" || a.integrationUI.Error != "" || a.errorText != "" || a.state.Integrations.Cloud.State != "ready" {
		t.Fatalf("late failure polluted the new visit: page=%q integrations=%q global=%q", a.cloudPage.error, a.integrationUI.Error, a.errorText)
	}
}

func TestCloudSettingsStatusRetryCanEnableAfterFreshRead(t *testing.T) {
	a, service := cloudNavigationTestApp(t)
	service.adjustSnapshot = func(snapshot *contracts.Snapshot) {
		snapshot.Integrations.Cloud.LoadState = contracts.LoadError
		snapshot.Integrations.Cloud.Error = "Engine not connected"
	}
	a.OpenRoute("settings/Cloud")
	tt := ui.NewTester(a.View, 1320, 1000)
	service.adjustSnapshot = nil
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, []string{"cloud.refresh", "cloud.refresh", "cloud.enable"}) {
		t.Fatalf("Retry did not follow the fresh off status: %v", got)
	}
	if a.state.Integrations.Cloud.State != "ready" {
		t.Fatal("Retry did not reach the stateful backend")
	}
}

func TestCloudSettingsNavigationWaitsForLoadingStatusToBecomeReady(t *testing.T) {
	a, service := cloudNavigationTestApp(t)
	service.adjustSnapshot = func(snapshot *contracts.Snapshot) { snapshot.Integrations.Cloud.LoadState = contracts.LoadLoading }
	a.OpenRoute("settings/Cloud")
	tt := ui.NewTester(a.View, 1320, 1000)
	if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, []string{"cloud.refresh"}) {
		t.Fatalf("loading status enabled Cloud before it was ready: %v", got)
	}
	service.adjustSnapshot = nil
	// Simulate the backend completing this load with a newer owned snapshot.
	if err := service.Service.Execute(a.ctx, contracts.Command{Kind: "cloud.refresh"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Service.Snapshot(a.ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.applyEvent(contracts.Event{Snapshot: &snapshot})
	tt.Frame()
	if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, []string{"cloud.refresh", "cloud.enable"}) {
		t.Fatalf("ready result did not continue this visit: %v", got)
	}
	for range 3 {
		tt.Frame()
	}
	if len(service.commands) != 2 || a.state.Integrations.Cloud.State != "ready" {
		t.Fatal("ready completion was duplicated or did not enable the mock")
	}
}

func TestCloudSettingsNavigationDoesNotRepeatEnableForOffReply(t *testing.T) {
	a, service := cloudNavigationTestApp(t)
	service.adjustSnapshot = func(snapshot *contracts.Snapshot) {
		if len(service.commands) > 0 && service.commands[len(service.commands)-1].Kind == "cloud.enable" {
			snapshot.Integrations.Cloud.State = "off"
			snapshot.Integrations.Cloud.DeviceID = ""
		}
	}
	a.OpenRoute("settings/Cloud")
	tt := ui.NewTester(a.View, 1320, 1000)
	for range 3 {
		a.OpenRoute("settings/Cloud")
		tt.Frame()
	}
	if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, []string{"cloud.refresh", "cloud.enable"}) {
		t.Fatalf("unexpected Off enable reply repeated the mutation: %v", got)
	}
	if a.state.Integrations.Cloud.State != "off" || a.cloudPage.busy != "" || a.cloudPage.waitingStatus {
		t.Fatal("unexpected Off reply did not settle the attempt")
	}
}

func TestCloudSettingsNavigationReentersAfterDirectChatRouteWithoutView(t *testing.T) {
	for _, test := range []struct {
		name    string
		pending string
		leave   func(*App)
		reenter func(*App)
	}{
		{
			name:    "session pick during enable then explicit Cloud",
			pending: "cloud.enable",
			leave:   func(a *App) { a.selectSession("chat-0") },
			reenter: func(a *App) { a.OpenRoute("settings/Cloud") },
		},
		{
			name:    "session pick during read then remembered Cloud",
			pending: "cloud.refresh",
			leave:   func(a *App) { a.selectSession("chat-0") },
			reenter: func(a *App) { a.OpenRoute("settings") },
		},
		{
			name:    "annotation during enable then settings navigation",
			pending: "cloud.enable",
			leave:   func(a *App) { a.attachAnnotation(contracts.Annotation{Title: "Selected button", Element: "button"}) },
			reenter: func(a *App) { a.navigate("settings") },
		},
		{
			name:    "session pick during enable then history return",
			pending: "cloud.enable",
			leave:   func(a *App) { a.selectSession("chat-0") },
			reenter: func(a *App) {
				a.history, a.historyIndex = []string{"settings/Cloud", "chat"}, 1
				a.back(-1)
			},
		},
		{
			name:    "new conversation already uses routed exit",
			pending: "cloud.enable",
			leave:   func(a *App) { a.NewConversation() },
			reenter: func(a *App) { a.OpenRoute("settings") },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, service := cloudNavigationTestApp(t)
			service.failKind, service.executeErr = test.pending, errors.New("old visit failed")
			var oldContext context.Context
			var oldGeneration uint64
			service.onExecute = func(ctx context.Context, command contracts.Command) {
				if command.Kind != test.pending {
					return
				}
				oldContext, oldGeneration = ctx, a.cloudPage.generation
				// Execute has captured the old failure. Disable this hook before
				// entering the replacement visit so nested requests cannot recurse.
				service.onExecute, service.executeErr = nil, nil
				test.leave(a)
				if a.route != "chat" {
					t.Fatalf("test did not exercise a real chat exit: %s", a.route)
				}
				// Deliberately do not build View between leaving and returning.
				test.reenter(a)
			}
			a.OpenRoute("settings/Cloud")
			want := []string{"cloud.refresh", "cloud.enable", "cloud.refresh", "cloud.enable"}
			if test.pending == "cloud.refresh" {
				want = []string{"cloud.refresh", "cloud.refresh", "cloud.enable"}
			}
			if got := cloudNavigationKinds(t, service); !reflect.DeepEqual(got, want) {
				t.Fatalf("return reused the old visit instead of refreshing: got %v, want %v", got, want)
			}
			if oldContext == nil || !errors.Is(oldContext.Err(), context.Canceled) || a.cloudPage.generation <= oldGeneration {
				t.Fatal("return did not cancel and replace the previous Cloud visit")
			}
			if a.route != "settings" || a.section != "Cloud" || a.state.Integrations.Cloud.State != "ready" || a.cloudPage.error != "" || a.cloudPage.busy != "" {
				t.Fatalf("old request polluted the replacement visit: route=%s section=%s status=%s page=%+v", a.route, a.section, a.state.Integrations.Cloud.State, a.cloudPage)
			}
		})
	}
}
