package main

import (
	"fmt"
	"strings"
)

func (c *Config) windowsInstallName() string {
	if c.Windows.InstallName != "" {
		return c.Windows.InstallName
	}
	return c.Name
}

func (c *Config) macBundleName() string {
	if c.MacOS.BundleName != "" {
		return fsName(c.MacOS.BundleName)
	}
	return c.executableName()
}

func (c *Config) macBundleFingerprint() string {
	return "\x00macos.bundle=" + c.macBundleName() + ".app\x00"
}

func (c *Config) validateDistributionNames() error {
	for _, field := range []struct{ key, name string }{{"windows.installName", c.Windows.InstallName}, {"macos.bundleName", c.MacOS.BundleName}} {
		if field.name == "" {
			continue
		}
		if field.name == "." || field.name == ".." || field.name != fsName(field.name) || strings.HasSuffix(field.name, ".") || field.name == " " {
			return fmt.Errorf("%s %q must be a single file name", field.key, field.name)
		}
	}
	if strings.HasSuffix(strings.ToLower(c.MacOS.BundleName), ".app") {
		return fmt.Errorf("macos.bundleName must omit the .app extension")
	}
	name := strings.ToUpper(strings.SplitN(c.Windows.InstallName, ".", 2)[0])
	if name == "CON" || name == "PRN" || name == "AUX" || name == "NUL" || len(name) == 4 && (strings.HasPrefix(name, "COM") || strings.HasPrefix(name, "LPT")) && name[3] >= '1' && name[3] <= '9' {
		return fmt.Errorf("windows.installName %q is a reserved Windows file name", c.Windows.InstallName)
	}
	return nil
}
