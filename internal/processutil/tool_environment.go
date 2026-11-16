package processutil

import "strings"

// ToolEnvironment inherits only operating-system and development-tool runtime
// settings. Application credentials require an explicit child-only layer.
// This prevents ambient provider/cloud secrets from leaking into tools/hooks;
// it is not a filesystem or process sandbox.
func ToolEnvironment(parent []string) []string {
	var result []string
	for _, entry := range parent {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		key := strings.ToUpper(name)
		switch key {
		case "PATH", "PATHEXT", "HOME", "USER", "LOGNAME", "USERPROFILE",
			"SYSTEMROOT", "WINDIR", "COMSPEC", "SYSTEMDRIVE", "HOMEDRIVE", "HOMEPATH",
			"APPDATA", "LOCALAPPDATA", "PROGRAMDATA", "PROGRAMFILES", "PROGRAMFILES(X86)",
			"TEMP", "TMP", "TMPDIR", "LANG", "LANGUAGE", "TZ", "TERM", "COLORTERM",
			"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "XDG_CACHE_HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME",
			"SSL_CERT_FILE", "SSL_CERT_DIR", "CURL_CA_BUNDLE", "REQUESTS_CA_BUNDLE", "NODE_EXTRA_CA_CERTS",
			"GOPATH", "GOROOT", "GOCACHE", "GOMODCACHE", "GOTOOLCHAIN", "GOPROXY", "GOSUMDB", "GOPRIVATE", "GONOPROXY", "GONOSUMDB",
			"JAVA_HOME", "ANDROID_HOME", "ANDROID_SDK_ROOT", "DEVELOPER_DIR", "SDKROOT",
			"VIRTUAL_ENV", "CONDA_PREFIX", "NVM_DIR", "VOLTA_HOME", "PNPM_HOME",
			"CARGO_HOME", "RUSTUP_HOME", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			result = append(result, entry)
		default:
			if strings.HasPrefix(key, "LC_") {
				result = append(result, entry)
			}
		}
	}
	return result
}
