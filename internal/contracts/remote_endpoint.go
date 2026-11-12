package contracts

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// ValidateRemoteURL enforces the transport policy shared by the client settings
// and the transport boundary: a remote host must use HTTPS unless it names the
// local loopback interface, where a plaintext port is normally an SSH tunnel or
// a reverse proxy on the same machine. It returns the trimmed address.
func ValidateRemoteURL(raw string) (string, error) {
	address := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("remote host must be an HTTP(S) URL")
	}
	if parsed.Scheme == "http" && !loopbackHost(parsed.Host) {
		return "", errors.New("remote host must use HTTPS unless it points at the local loopback interface")
	}
	return address, nil
}

func loopbackHost(host string) bool {
	name := host
	if nameOnly, _, err := net.SplitHostPort(host); err == nil {
		name = nameOnly
	}
	name = strings.Trim(name, "[]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	address, err := netip.ParseAddr(name)
	return err == nil && address.IsLoopback()
}
