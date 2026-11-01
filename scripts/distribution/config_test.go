package distribution

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func replica() Config {
	return Config{Name: "GCode", Identifier: "dev.gcode.rcode", Version: "0.1.0"}
}

func TestExternalNamesDoNotRenameExecutable(t *testing.T) {
	c := replica()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.Executable("windows") != "GCode.exe" || filepath.ToSlash(c.Executable("darwin")) != "GCode.app/Contents/MacOS/GCode" || c.WindowsInstallDirectory() != `%LOCALAPPDATA%\Programs\GCode` {
		t.Fatal("external and internal app identities mixed")
	}
}

func TestUnsafeDistributionConfigurationIsRejected(t *testing.T) {
	for _, name := range []string{"empty name", "invalid name", "changed app name"} {
		t.Run(name, func(t *testing.T) {
			c := replica()
			switch name {
			case "empty name":
				c.Name = ""
			case "invalid name":
				c.Name = "GCode/"
			case "changed app name":
				c.Name = "GCode Native UI"
			}
			if c.Validate() == nil {
				t.Fatal("unsafe distribution configuration accepted")
			}
		})
	}
}

func TestProjectProductNamesAreGCode(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "mygo.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "GCode" {
		t.Fatal("project distribution names differ from GCode")
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
	bad := strings.Replace(good, "<key>CFBundleDisplayName</key><string>GCode</string>", "<key>CFBundleDisplayName</key><string>GCode Native UI</string>", 1)
	if err := os.WriteFile(path, []byte(bad), 0644); err != nil {
		t.Fatal(err)
	}
	if c.VerifyInfoPlist(path) == nil {
		t.Fatal("external bundle name replaced the application display identity")
	}
}
