package distribution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func replica() Config {
	c := Config{Name: "Zeron", Identifier: "dev.gcode.zeronui", Version: "0.1.0"}
	c.MacOS.BundleName = "Zeron Native UI"
	c.Windows.InstallName = "Zeron Native UI"
	return c
}

func TestExternalNamesDoNotRenameExecutable(t *testing.T) {
	c := replica()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Executable("windows") != "Zeron.exe" || filepath.ToSlash(c.Executable("darwin")) != "Zeron Native UI.app/Contents/MacOS/Zeron" || c.WindowsInstallDirectory() != `%LOCALAPPDATA%\Programs\Zeron Native UI` {
		t.Fatal("external and internal app identities mixed")
	}
	c.MacOS.BundleName = "Independent Bundle"
	if filepath.ToSlash(c.Executable("darwin")) != "Independent Bundle.app/Contents/MacOS/Zeron" {
		t.Fatal("bundle path was hard coded")
	}
}

func TestUnsafeCoexistenceConfigurationIsRejected(t *testing.T) {
	for _, name := range []string{"same windows name", "case collision", "same bundle name", "bundle traversal", "changed app name"} {
		t.Run(name, func(t *testing.T) {
			c := replica()
			switch name {
			case "same windows name":
				c.Windows.InstallName = "Zeron"
			case "case collision":
				c.Windows.InstallName = "zeron"
			case "same bundle name":
				c.MacOS.BundleName = "Zeron"
			case "bundle traversal":
				c.MacOS.BundleName = "../Original"
			case "changed app name":
				c.Name = "Zeron Native UI"
			}
			if c.Validate() == nil {
				t.Fatal("unsafe coexistence configuration accepted")
			}
		})
	}
}

func TestLegacyBundleCannotEnterRelease(t *testing.T) {
	dir := t.TempDir()
	c := replica()
	if err := c.CheckNoLegacyBundle(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "Zeron.app"), 0755); err != nil {
		t.Fatal(err)
	}
	if c.CheckNoLegacyBundle(dir) == nil {
		t.Fatal("old collision-prone bundle accepted")
	}
}

func TestProjectNamesAreExplicitlyIndependent(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Windows.InstallName != "Zeron Native UI" || c.MacOS.BundleName != "Zeron Native UI" {
		t.Fatal("project distribution names differ from the chosen independent identity")
	}
}

func TestBundleDisplayIdentityCannotFollowExternalFileName(t *testing.T) {
	c := replica()
	var body strings.Builder
	for key, value := range map[string]string{"CFBundleName": c.Name, "CFBundleDisplayName": c.Name, "CFBundleExecutable": c.Name, "CFBundleIdentifier": c.Identifier, "CFBundleVersion": c.Version, "CFBundleShortVersionString": c.Version} {
		body.WriteString("<key>" + key + "</key><string>" + value + "</string>")
	}
	path := filepath.Join(t.TempDir(), "Info.plist")
	good := "<plist><dict>" + body.String() + "</dict></plist>"
	if err := os.WriteFile(path, []byte(good), 0644); err != nil {
		t.Fatal(err)
	}
	if err := c.VerifyInfoPlist(path); err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(good, "<key>CFBundleDisplayName</key><string>Zeron</string>", "<key>CFBundleDisplayName</key><string>Zeron Native UI</string>", 1)
	if err := os.WriteFile(path, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	if c.VerifyInfoPlist(path) == nil {
		t.Fatal("external bundle name replaced the application display identity")
	}
}
