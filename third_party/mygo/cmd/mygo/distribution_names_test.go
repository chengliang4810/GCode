package main

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDistributionNamesKeepDisplayAndExecutableNames(t *testing.T) {
	c := &Config{Name: "Zeron", root: t.TempDir()}
	c.applyDefaults()
	if c.windowsInstallName() != "Zeron" || c.macBundleName() != "Zeron" {
		t.Fatal("default packaging names changed")
	}
	c.Windows.InstallName = "Zeron Native UI"
	c.MacOS.BundleName = "Zeron Native UI"
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	if c.windowsInstallName() != "Zeron Native UI" || c.macBundleName() != "Zeron Native UI" || c.Name != "Zeron" || c.executableName() != "Zeron" {
		t.Fatal("install identity changed application identity")
	}
}

func TestDevelopmentFingerprintTracksOnlyBundleNameChange(t *testing.T) {
	c := &Config{Name: "Zeron"}
	first := sha256.Sum256([]byte(c.macBundleFingerprint()))
	c.MacOS.BundleName = "Zeron"
	if got := sha256.Sum256([]byte(c.macBundleFingerprint())); got != first {
		t.Fatal("explicit default bundle name changed cache identity")
	}
	c.MacOS.BundleName = "Zeron Native UI"
	if got := sha256.Sum256([]byte(c.macBundleFingerprint())); got == first {
		t.Fatal("bundle name change did not invalidate development build identity")
	}
}

func TestDistributionNamesRejectPathsAndReservedWindowsNames(t *testing.T) {
	for _, tc := range []struct{ name, windows, mac string }{
		{"windows traversal", "../Zeron", ""}, {"windows path", `C:\Zeron`, ""},
		{"windows reserved", "CON", ""}, {"windows reserved extension", "lpt1.txt", ""},
		{"windows trailing dot", "Zeron.", ""}, {"mac traversal", "", "../Zeron"},
		{"mac extension", "", "Zeron.app"}, {"mac whitespace", "", "  "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{Name: "Zeron", Windows: Windows{InstallName: tc.windows}, MacOS: MacOS{BundleName: tc.mac}}
			if err := c.validate(); err == nil {
				t.Fatal("unsafe distribution name accepted")
			}
		})
	}
}

func TestBundleCoexistsWithoutRenamingApplication(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "Zeron.app", "Contents", "original.txt")
	if err := os.MkdirAll(filepath.Dir(original), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "replica-bin")
	if err := os.WriteFile(bin, []byte("replica"), 0755); err != nil {
		t.Fatal(err)
	}
	c := &Config{Name: "Zeron", Identifier: "dev.gcode.zeronui", root: dir, MacOS: MacOS{BundleName: "Zeron Native UI"}}
	c.applyDefaults()
	app, err := writeBundle(c, dir, bin, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(app) != "Zeron Native UI.app" {
		t.Fatalf("bundle name = %q", app)
	}
	if data, err := os.ReadFile(original); err != nil || string(data) != "original" {
		t.Fatal("original bundle changed")
	}
	if data, err := os.ReadFile(bundleExecutable(c, app)); err != nil || string(data) != "replica" {
		t.Fatal("bundle executable was renamed or lost")
	}
	data, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{"CFBundleName", "CFBundleDisplayName", "CFBundleExecutable"} {
		if !strings.Contains(string(data), "<key>"+entry+"</key>\n\t<string>Zeron</string>") {
			t.Fatalf("%s no longer identifies Zeron", entry)
		}
	}
	if !strings.Contains(string(data), "<string>dev.gcode.zeronui</string>") {
		t.Fatal("independent bundle identifier lost")
	}
	found := false
	for _, record := range dmgRecords(filepath.Base(app)) {
		if record.name == "Zeron.app" {
			t.Fatal("DMG targets the original bundle name")
		}
		if record.name == "Zeron Native UI.app" && record.code == "Iloc" {
			found = true
		}
	}
	if !found {
		t.Fatal("DMG icon layout lost the distinct app target")
	}
}

// This test only compiles the installer and inspects its input script. It never
// installs, launches or uninstalls an app, or modifies real shortcuts/registry.
func TestWindowsInstallerCoexistenceScript(t *testing.T) {
	if makensis() == "" {
		t.Skip("NSIS compiler is not installed")
	}
	stage, work := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(stage, "Zeron.exe"), []byte("test payload, never executed"), 0644); err != nil {
		t.Fatal(err)
	}
	c := &Config{Name: "Zeron", Identifier: "dev.gcode.zeronui", root: stage, Windows: Windows{InstallName: "Zeron Native UI"}}
	c.applyDefaults()
	if _, err := writeInstaller(c, stage, work, "Zeron.exe", "amd64", []string{"Zeron.exe"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(work, "installer.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{
		`Name "Zeron"`, `InstallDir "$LOCALAPPDATA\Programs\Zeron Native UI"`,
		`CreateShortCut "$SMPROGRAMS\Zeron Native UI.lnk" "$INSTDIR\Zeron.exe"`,
		`CreateShortCut "$DESKTOP\Zeron Native UI.lnk" "$INSTDIR\Zeron.exe"`,
		`"DisplayName" "Zeron Native UI"`, `Push "$SMPROGRAMS\Zeron Native UI.lnk"`,
		`Push "$DESKTOP\Zeron Native UI.lnk"`, `Uninstall\dev.gcode.zeronui`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("installer script lacks %s", want)
		}
	}
	for _, unsafe := range []string{`InstallDir "$LOCALAPPDATA\Programs\Zeron"`, `$SMPROGRAMS\Zeron.lnk`, `$DESKTOP\Zeron.lnk`} {
		if strings.Contains(script, unsafe) {
			t.Fatalf("installer still touches original location: %s", unsafe)
		}
	}
}
