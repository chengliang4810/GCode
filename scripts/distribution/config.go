// Package distribution describes the application's installation identity.
package distribution

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
}

func Load(path string) (Config, error) {
	var config Config
	data, err := os.ReadFile(path)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	return config, config.Validate()
}

func (c Config) Validate() error {
	if c.Name != "GCode" || c.Identifier != "dev.gcode.rcode" || c.Version == "" || strings.ContainsAny(c.Version, "/\\") {
		return fmt.Errorf("unexpected application identity")
	}
	if c.Name == "" || c.Name != strings.TrimSpace(c.Name) || strings.ContainsAny(c.Name, "/\\:*?\"<>|\r\n") || strings.HasSuffix(c.Name, ".") {
		return fmt.Errorf("application installation names must be valid")
	}
	return nil
}

func (c Config) BundleFile() string { return c.Name + ".app" }

func (c Config) Executable(goos string) string {
	if goos == "darwin" {
		return filepath.Join(c.BundleFile(), "Contents", "MacOS", c.Name)
	}
	return c.Name + ".exe"
}

func (c Config) Resources(dir, goos string) string {
	if goos == "darwin" {
		return filepath.Join(dir, c.BundleFile(), "Contents", "Resources")
	}
	return dir
}

func (c Config) WindowsInstallDirectory() string {
	return `%LOCALAPPDATA%\Programs\` + c.Name
}

func (c Config) VerifyInfoPlist(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var plist struct {
		Dict struct {
			Entries []struct {
				XMLName xml.Name
				Text    string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"dict"`
	}
	if err := xml.Unmarshal(data, &plist); err != nil {
		return err
	}
	want := map[string]string{"CFBundleName": c.Name, "CFBundleDisplayName": c.Name, "CFBundleExecutable": c.Name, "CFBundleIdentifier": c.Identifier, "CFBundleVersion": c.Version, "CFBundleShortVersionString": c.Version}
	seen := map[string]bool{}
	for i, entry := range plist.Dict.Entries {
		if entry.XMLName.Local != "key" {
			continue
		}
		value, needed := want[entry.Text]
		if !needed {
			continue
		}
		if seen[entry.Text] || i+1 >= len(plist.Dict.Entries) || plist.Dict.Entries[i+1].XMLName.Local != "string" || plist.Dict.Entries[i+1].Text != value {
			return fmt.Errorf("unexpected macOS application identity: %s", entry.Text)
		}
		seen[entry.Text] = true
	}
	if len(seen) != len(want) {
		return fmt.Errorf("macOS application identity is incomplete")
	}
	return nil
}
