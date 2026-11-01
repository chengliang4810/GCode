package desktop

import (
	"slices"
	"strings"
	"testing"

	"gcode/internal/contracts"
	"github.com/egoist/mygo/ui"
)

func TestComposerCloudStandardAuthorizationUIAndSend(t *testing.T) {
	a := testApp(t, "default")
	tt := ui.NewTester(a.View, 1320, 1000)
	footerGitClick(t, tt, "Select device")
	if _, ok := tt.Find("Cloud"); ok {
		t.Fatal("unreachable Cloud repository was offered before authorization")
	}
	// Cloud gating must not remove ordinary remote devices.
	footerGitClick(t, tt, "MacBook Pro")
	if a.deviceID != "macbook" {
		t.Fatal("Cloud filtering hid or rejected an ordinary remote device")
	}
	footerGitClick(t, tt, "Select device")
	footerGitClick(t, tt, "This device")
	a.pickComposerDevice("cloud-1")
	if a.deviceID != "local" {
		t.Fatal("a stale Cloud row bypassed the current repository guard")
	}

	// Exercise the existing Settings controls and typed backend, without
	// injecting a repository or a Cloud session into the UI snapshot.
	a.OpenRoute("settings/Cloud")
	tt.Frame()
	integrationClick(t, tt, "Connect")
	if a.state.Integrations.Cloud.Github.State != "connected" || len(a.state.Integrations.Cloud.Repositories) != 1 || !tt.HasText("r-code/fieldnotes") {
		t.Fatal("standard GitHub connect did not publish the project's canonical repository")
	}
	integrationClick(t, tt, "Choose on GitHub")
	if len(a.state.Integrations.Cloud.Repositories) != 2 || !tt.HasText("r-code/rcode") {
		t.Fatal("standard repository selection did not publish canonical repositories")
	}
	a.newConversation()
	tt.Frame()
	footerGitClick(t, tt, "Select device")
	footerGitClick(t, tt, "Cloud")
	if a.deviceID != "cloud-1" || a.footerGit.Kind != contracts.CheckoutCloud || a.centralControls().modelNoAgents() {
		t.Fatal("authorized Cloud choice did not retarget the composer and agents")
	}
	if _, ok := tt.Find("Select checkout"); ok {
		t.Fatal("Cloud draft exposed a local checkout picker")
	}
	resource := a.state.RepoRefs[a.footerGit.Scope.Key()]
	if resource.State != contracts.LoadReady || resource.Scope.CloudRepository != "r-code/fieldnotes" || len(resource.Rows) == 0 {
		t.Fatalf("Cloud choice did not load typed Cloud refs: %+v", resource)
	}
	for _, row := range resource.Rows {
		if row.WorktreePath != "" {
			t.Fatal("Cloud branch rows borrowed local worktrees")
		}
	}
	footerGitClick(t, tt, "Select ref")
	footerGitClick(t, tt, "Ref fieldnotes/settings-search")
	footerGitClick(t, tt, "Message")
	tt.Type("Create an independent Cloud checkout")
	tt.Key(0, ui.KeyEnter)
	session := a.currentSession()
	if session.ID == "" || session.DeviceID != "cloud-"+session.ID || session.CWD != "/workspace/Fieldnotes" || session.Branch != "fieldnotes/settings-search" || session.SourceContext == nil {
		t.Fatalf("standard Cloud UI did not send into a new host checkout: %+v; error=%s", session, a.errorText)
	}
	if a.centralControls().modelNoAgents() {
		t.Fatal("the new Cloud machine lost its provider capabilities")
	}
	createdDevice := session.DeviceID
	a.newConversation()
	tt.Frame()
	footerGitClick(t, tt, "Select device")
	footerGitClick(t, tt, "This device")
	a.pickComposerDevice(createdDevice)
	if a.deviceID != "local" {
		t.Fatal("a per-session Cloud machine was accepted as a draft destination")
	}
	footerGitClick(t, tt, "Select device")
	cloudRows := 0
	for _, text := range tt.Texts() {
		if text == "Cloud" {
			cloudRows++
		}
	}
	// One menu choice contributes its accessible label and native text.
	if cloudRows != 2 {
		t.Fatalf("device menu exposed per-session machines beside the one virtual Cloud row: %d", cloudRows)
	}
	footerGitClick(t, tt, "Cloud")
	a.OpenRoute("settings/Cloud")
	tt.Frame()
	integrationClick(t, tt, "Disconnect")
	a.newConversation()
	tt.Frame()
	if a.deviceID != "local" || a.footerGit.Kind != contracts.CheckoutCurrent || a.footerGit.Branch != "" {
		t.Fatalf("repository loss left a Cloud chip/local plan mismatch: device=%s footer=%+v", a.deviceID, a.footerGit)
	}
	footerGitClick(t, tt, "Select device")
	if _, ok := tt.Find("Cloud"); ok {
		t.Fatal("disconnected Cloud repository remained selectable")
	}
	if strings.HasPrefix(a.deviceID, "cloud-") {
		t.Fatal("disconnected Cloud draft retained a machine target")
	}
}

func TestComposerCloudRepositoryGuardUsesFullNameAndEnabledState(t *testing.T) {
	a := testApp(t, "default")
	project := a.state.Projects[0]
	for _, tc := range []struct {
		name, cloudState, repoID, repoName string
		want                               bool
	}{
		{"canonical ASCII case", "ready", "fieldnotes", "R-CODE/FIELDNOTES", true},
		{"ID alone is not a canonical repository", "ready", project.CloudRepository, "other/fieldnotes", false},
		{"off with a stale cache", "off", "fieldnotes", project.CloudRepository, false},
		{"provisioning with a stale cache", "provisioning", "fieldnotes", project.CloudRepository, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := a.state
			state.Integrations.Cloud.State = tc.cloudState
			state.Integrations.Cloud.Repositories = []contracts.CloudRepository{{ID: tc.repoID, Name: tc.repoName, Authorized: true}}
			if got := footerCloudRepositoryAvailable(state, project); got != tc.want {
				t.Fatalf("repository availability = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCanvasCloudPickSurvivesOnlySameProjectAndHost(t *testing.T) {
	for _, tc := range []struct {
		name, sessionID string
		wantCloud       bool
	}{
		{name: "same project and host", sessionID: "chat-0", wantCloud: true},
		{name: "different project and host", sessionID: "chat-6"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, "default")
			tt := ui.NewTester(a.View, 1320, 1000)
			a.OpenRoute("settings/Cloud")
			tt.Frame()
			integrationClick(t, tt, "Connect")
			a.newConversation()
			tt.Frame()
			a.pickComposerDevice("cloud-1")
			tt.Frame()
			if a.footerGit.Kind != contracts.CheckoutCloud {
				t.Fatal("fixture did not select a Cloud checkout")
			}
			a.selectSession(tc.sessionID)
			tt.Frame()
			a.selectSession("")
			tt.Frame()
			if (a.footerGit.Kind == contracts.CheckoutCloud) != tc.wantCloud || footerCloudDevice(a.deviceID) != tc.wantCloud || a.projectID != "fieldnotes" {
				t.Fatalf("wrong Cloud restoration: target=%q device=%q kind=%q", a.projectID, a.deviceID, a.footerGit.Kind)
			}
		})
	}
}

func TestCanvasCloudPickClearsWhenRememberedProjectOrHostChanges(t *testing.T) {
	for _, replaceProject := range []bool{false, true} {
		t.Run(map[bool]string{false: "same project changes host", true: "fallback to another authorized project"}[replaceProject], func(t *testing.T) {
			a := testApp(t, "default")
			tt := ui.NewTester(a.View, 1320, 1000)
			a.OpenRoute("settings/Cloud")
			tt.Frame()
			integrationClick(t, tt, "Connect")
			integrationClick(t, tt, "Choose on GitHub")
			a.newConversation()
			tt.Frame()
			a.pickComposerDevice("cloud-1")
			tt.Frame()
			a.selectSession("chat-0")
			tt.Frame()
			snapshot, err := a.service.Snapshot(a.ctx)
			if err != nil {
				t.Fatal(err)
			}
			snapshot.Revision = a.state.Revision + 1
			snapshot.Sessions = slices.DeleteFunc(snapshot.Sessions, func(s contracts.Session) bool { return s.ID == "chat-0" })
			wantProject, wantDevice := "fieldnotes", "remote"
			if replaceProject {
				snapshot.Projects = slices.DeleteFunc(snapshot.Projects, func(p contracts.Project) bool { return p.ID == "fieldnotes" })
				for i := range snapshot.Projects {
					switch snapshot.Projects[i].ID {
					case "backend":
						snapshot.Projects[i].Name = "Alpha remote"
					case "rcode":
						snapshot.Projects[i].Name = "Zulu local"
					}
				}
				wantProject, wantDevice = "rcode", "local"
			} else {
				for i := range snapshot.Projects {
					if snapshot.Projects[i].ID == "fieldnotes" {
						snapshot.Projects[i].DeviceID = "remote"
					}
				}
			}
			a.apply(snapshot)
			tt.Frame()
			if a.sessionID != "" || a.projectID != wantProject || a.deviceID != wantDevice || a.footerGit.Kind == contracts.CheckoutCloud {
				t.Fatalf("Cloud silently returned after target changed: id=%q project=%q device=%q kind=%q", a.sessionID, a.projectID, a.deviceID, a.footerGit.Kind)
			}
		})
	}
}
