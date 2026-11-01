// Package repohost derives browser links from Git remotes without credentials.
package repohost

import (
	"net/url"
	"strings"
)

func PullRequestsURL(remote string) string {
	remote = strings.TrimSpace(remote)
	if strings.HasPrefix(remote, "git@") {
		host, path, ok := strings.Cut(strings.TrimPrefix(remote, "git@"), ":")
		if !ok {
			return ""
		}
		remote = "ssh://" + host + "/" + path
	}
	u, err := url.Parse(remote)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	if u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ssh" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "\\ \r\n") {
			return ""
		}
	}
	var suffix string
	switch host {
	case "github.com":
		if len(parts) != 2 {
			return ""
		}
		suffix = "/pulls"
	case "gitlab.com":
		suffix = "/-/merge_requests"
	case "bitbucket.org":
		if len(parts) != 2 {
			return ""
		}
		suffix = "/pull-requests"
	default:
		return ""
	}
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "https://" + host + "/" + strings.Join(parts, "/") + suffix
}
