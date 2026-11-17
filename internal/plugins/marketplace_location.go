package plugins

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Locations are resolved on the business host, including local paths in remote mode.
func parseMarketplaceLocation(location string) (Source, error) {
	location = strings.TrimSpace(location)
	if location == "" || strings.ContainsAny(location, "\r\n\x00") {
		return Source{}, errors.New("a marketplace location is required")
	}
	if strings.HasPrefix(location, "~/") || strings.HasPrefix(location, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return Source{}, err
		}
		location = filepath.Join(home, location[2:])
	}
	if info, err := os.Stat(location); err == nil {
		path, err := filepath.Abs(location)
		if err != nil {
			return Source{}, err
		}
		if info.IsDir() {
			return Source{Source: "directory", Path: path}, nil
		}
		if info.Mode().IsRegular() {
			return Source{Source: "file", Path: path}, nil
		}
		return Source{}, errors.New("marketplace location must be a file or directory")
	}
	if filepath.IsAbs(location) || strings.HasPrefix(location, ".") {
		return Source{}, errors.New("marketplace file or directory does not exist on the host")
	}
	if strings.HasPrefix(location, "git@") {
		if err := validateGitAddress(location); err != nil {
			return Source{}, err
		}
		return Source{Source: "git", URL: location}, nil
	}
	address, err := url.Parse(location)
	if err != nil {
		return Source{}, err
	}
	if address.Scheme != "" {
		if err := validateGitAddress(location); err != nil {
			return Source{}, err
		}
		if (address.Scheme == "https" || address.Scheme == "http") &&
			strings.EqualFold(filepath.Ext(address.Path), ".json") {
			return Source{Source: "url", URL: location}, nil
		}
		return Source{Source: "git", URL: location}, nil
	}
	repo := strings.TrimSuffix(strings.TrimSuffix(location, "/"), ".git")
	source := Source{Source: "github", Repo: repo}
	if _, err := gitAddress(source); err != nil {
		return Source{}, errors.New("use a GitHub owner/repository, Git URL, or existing file or directory")
	}
	return source, nil
}

func validateMarketplacePluginSource(marketplace, plugin Source) error {
	if plugin.Relative != "" {
		if marketplace.Source == "directory" || marketplace.Source == "git" ||
			marketplace.Source == "github" || marketplace.Source == "git-subdir" {
			return nil
		}
		return errors.New("marketplace entries must use remote plugin sources")
	}

	if marketplace.Source == "directory" || marketplace.Source == "settings" {
		return validatePluginSourceType(plugin)
	}
	switch plugin.Source {
	case "directory", "file", "settings":
		return errors.New("marketplace entries must use remote plugin sources")
	case "github", "git", "url", "git-subdir":
		if localGitSource(plugin) {
			return errors.New("remote marketplace entries cannot use local plugin sources")
		}
	default:
		return validatePluginSourceType(plugin)
	}
	return nil
}

func validatePluginSourceType(source Source) error {
	switch source.Source {
	case "github", "git", "url", "git-subdir", "directory", "file", "settings":
		return nil
	case "":
		return errors.New("plugin source type is required")
	default:
		return errors.New("unsupported plugin source type")
	}
}

func localGitSource(source Source) bool {
	if source.Source == "github" {
		return false
	}
	address := strings.TrimSpace(source.URL)
	if filepath.IsAbs(address) || strings.HasPrefix(address, "./") || strings.HasPrefix(address, "../") ||
		strings.HasPrefix(address, "~/") || strings.HasPrefix(address, `~\`) {
		return true
	}
	u, err := url.Parse(address)
	return err == nil && strings.EqualFold(u.Scheme, "file")
}
