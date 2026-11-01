package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo"
)

func TestNativeDirectoriesSeparatePreferencesFromRuntimeData(t *testing.T) {
	config := filepath.Join(t.TempDir(), ".gcode")
	for _, runtimeRoot := range []string{config, filepath.Join(t.TempDir(), "运行数据")} {
		t.Run(filepath.Base(runtimeRoot), func(t *testing.T) {
			got := nativeDirectoryPaths(config, runtimeRoot, "")
			want := nativeDirectories{
				userData:  filepath.Join(config, "native"),
				cache:     filepath.Join(runtimeRoot, "cache", "gcode"),
				logs:      filepath.Join(runtimeRoot, "logs"),
				temp:      filepath.Join(runtimeRoot, "tmp", "gcode"),
				browser:   filepath.Join(runtimeRoot, "browser"),
				downloads: filepath.Join(runtimeRoot, "downloads"),
			}
			if got != want {
				t.Fatalf("native directories = %+v, want %+v", got, want)
			}
		})
	}
}

func TestConfigureNativeDirectoriesSetsRuntimeDownloads(t *testing.T) {
	previous := mygo.App
	mygo.App = &mygo.Application{}
	defer func() { mygo.App = previous }()
	config, runtimeRoot := t.TempDir(), t.TempDir()
	if err := configureNativeDirectories(config, runtimeRoot, "live"); err != nil {
		t.Fatal(err)
	}
	want := nativeDirectoryPaths(config, runtimeRoot, "live")
	for name, directory := range map[mygo.PathName]string{
		mygo.PathUserData: want.userData, mygo.PathCache: want.cache,
		mygo.PathLogs: want.logs, mygo.PathTemp: want.temp, mygo.PathDownloads: want.downloads,
	} {
		got, err := mygo.App.Path(name)
		if err != nil || got != directory {
			t.Fatalf("native %s = %q, %v; want %q", name, got, err, directory)
		}
		if info, err := os.Stat(directory); err != nil || !info.IsDir() {
			t.Fatalf("native %s directory unavailable: %v", name, err)
		}
	}
}

func TestNativeScenarioDirectoriesStayOutsideUserProfiles(t *testing.T) {
	config, runtimeRoot := t.TempDir(), t.TempDir()
	for _, scenario := range []string{"default", "live", "showcase"} {
		t.Run(scenario, func(t *testing.T) {
			got := nativeDirectoryPaths(config, runtimeRoot, scenario)
			want := nativeDirectories{
				userData:  filepath.Join(config, "acceptance", scenario, "native"),
				cache:     filepath.Join(runtimeRoot, "acceptance", scenario, "cache", "gcode"),
				logs:      filepath.Join(runtimeRoot, "acceptance", scenario, "logs"),
				temp:      filepath.Join(runtimeRoot, "acceptance", scenario, "tmp", "gcode"),
				browser:   filepath.Join(runtimeRoot, "acceptance", scenario, "browser"),
				downloads: filepath.Join(runtimeRoot, "acceptance", scenario, "downloads"),
			}
			if got != want {
				t.Fatalf("scenario directories = %+v, want %+v", got, want)
			}
		})
	}
}

func TestUnknownScenarioCannotBecomeAProfilePath(t *testing.T) {
	for _, scenario := range []string{"../GCode", "../../GCode", "C:\\GCode", "unknown"} {
		if validScenario(scenario) {
			t.Fatalf("invalid profile scenario accepted: %s", scenario)
		}
	}
	if !validScenario("showcase") || !validScenario("live") || !validScenario("transcript") || !validScenario("") {
		t.Fatal("normal scenarios rejected")
	}
}
