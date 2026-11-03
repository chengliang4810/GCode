package plugins

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var fullSHA = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

func resolveDependencyTag(ctx context.Context, source Source, name string, constraints []string) (result Source, found bool, resultErr error) {
	defer func() { resultErr = redactSourceError(resultErr, []string{source.URL}, nil) }()
	address, err := gitAddress(source)
	if err != nil {
		return source, false, err
	}
	output, err := git(ctx, "", "ls-remote", "--tags", "--refs", "--", address, "refs/tags/"+name+"--v*")
	if err != nil {
		return source, false, err
	}
	var chosen string
	var best semanticVersion
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		tag := strings.TrimPrefix(fields[1], "refs/tags/")
		version := strings.TrimPrefix(tag, name+"--v")
		parsed, err := parseVersion(version)
		if err != nil || checkVersions(version, constraints) != nil {
			continue
		}
		if chosen == "" || compareVersion(parsed, best) > 0 {
			chosen, best = tag, parsed
		}
	}
	if chosen == "" {
		return source, false, nil
	}
	// Fetch by tag and resolve the commit after peeling any annotated tag.
	source.Ref, source.SHA = chosen, ""
	return source, true, nil
}

// install prepares a dependency graph in isolation and publishes the registry once.
// Prepared immutable directories may remain after a failure; no active record changes.
func (m *Manager) install(ctx context.Context, marketplace, name string) (Installation, error) {
	next := copyValue(m.data)
	visiting := map[string]bool{}
	completed := map[string]Installation{}
	constraints := map[string][]string{}
	rootMarketplace := marketplace
	for _, catalog := range next.Catalogs {
		if catalog.SourceID == marketplace {
			rootMarketplace = catalog.Marketplace.Name
			break
		}
	}
	var crossAllowed []string
	for _, catalog := range next.Catalogs {
		if catalog.Marketplace.Name == rootMarketplace {
			crossAllowed = catalog.Marketplace.AllowCrossMarketplaceDependenciesOn
			break
		}
	}
	var visit func(string, string, string, bool) (Installation, error)
	visit = func(market, plugin, constraint string, root bool) (Installation, error) {
		if err := ctx.Err(); err != nil {
			return Installation{}, err
		}
		for _, catalog := range next.Catalogs {
			if market == catalog.SourceID {
				market = catalog.Marketplace.Name
				break
			}
		}
		id := plugin + "@" + market
		if visiting[id] {
			return Installation{}, fmt.Errorf("plugin dependency cycle reaches %s", id)
		}
		if constraint != "" && !slices.Contains(constraints[id], constraint) {
			constraints[id] = append(constraints[id], constraint)
		}
		if result, ok := completed[id]; ok && checkVersions(result.Version, constraints[id]) == nil {
			return result, nil
		}
		visiting[id] = true
		defer delete(visiting, id)
		var candidate Installation
		existing, exists := selectInstallations(next.Installations)[id]
		if !root && exists && existing.Enabled && checkVersions(existing.Version, constraints[id]) == nil {
			candidate = existing
		} else {
			if !root && market != rootMarketplace && !slices.Contains(crossAllowed, market) {
				return Installation{}, fmt.Errorf("cross-marketplace dependency %s is not in %s allowCrossMarketplaceDependenciesOn", id, rootMarketplace)
			}
			prepared, err := m.prepare(ctx, market, plugin, constraints[id])
			if err != nil {
				return Installation{}, fmt.Errorf("install %s: %w", id, err)
			}
			candidate = prepared
			if err := checkVersions(candidate.Version, constraints[id]); err != nil {
				return Installation{}, fmt.Errorf("dependency %s: %w", id, err)
			}
			for _, old := range next.Installations {
				if old.ID == candidate.ID {
					candidate.InstalledAt = old.InstalledAt
					if root {
						candidate.Enabled = old.Enabled
					}
					break
				}
			}
			if !root {
				candidate.Enabled = true
			}
		}
		for _, dependency := range candidate.Manifest.Dependencies {
			depName, depMarket, err := dependencyTarget(dependency, candidate.Marketplace)
			if err != nil {
				return Installation{}, fmt.Errorf("%s: %w", id, err)
			}
			if _, err := visit(depMarket, depName, dependency.Version, false); err != nil {
				return Installation{}, fmt.Errorf("%s requires %s: %w", id, dependency.Name, err)
			}
		}
		upsertInstallation(&next, candidate)
		completed[id] = candidate
		return candidate, nil
	}
	result, err := visit(marketplace, name, "", true)
	if err != nil {
		return Installation{}, err
	}
	if err := validateDependencyTransition(m.data, next); err != nil {
		return Installation{}, err
	}
	if err := m.commit(next); err != nil {
		return Installation{}, err
	}
	return copyValue(result), nil
}

func upsertInstallation(data *registry, candidate Installation) {
	for i, old := range data.Installations {
		if old.ID == candidate.ID {
			data.Installations[i] = candidate
			return
		}
	}
	data.Installations = append(data.Installations, candidate)
}

func dependencyTarget(dependency Dependency, ownMarketplace string) (string, string, error) {
	plugin, market := dependency.Name, ownMarketplace
	if at := strings.LastIndex(dependency.Name, "@"); at >= 0 {
		plugin, market = dependency.Name[:at], dependency.Name[at+1:]
	}
	if dependency.Marketplace != "" {
		market = dependency.Marketplace
	}
	if !validName.MatchString(plugin) || !validName.MatchString(market) {
		return "", "", fmt.Errorf("invalid dependency %q", dependency.Name)
	}
	return plugin, market, nil
}

func selectInstallations(installations []Installation) map[string]Installation {
	selected := make(map[string]Installation, len(installations))
	for _, installation := range installations {
		selected[installation.ID] = installation
	}
	return selected
}

func installationIssues(installation Installation, selected map[string]Installation, visiting map[string]bool) []Issue {
	if !installation.Enabled {
		return nil
	}
	if visiting[installation.ID] {
		return []Issue{{Component: "dependencies", Path: installation.ID, Message: "plugin dependency cycle"}}
	}
	visiting[installation.ID] = true
	defer delete(visiting, installation.ID)
	var issues []Issue
	for _, dependency := range installation.Manifest.Dependencies {
		name, market, err := dependencyTarget(dependency, installation.Marketplace)
		if err != nil {
			issues = append(issues, Issue{Component: "dependencies", Path: dependency.Name, Message: err.Error()})
			continue
		}
		id := name + "@" + market
		target, exists := selected[id]
		switch {
		case !exists:
			issues = append(issues, Issue{Component: "dependencies", Path: id, Message: "required plugin is not installed"})
		case !target.Enabled:
			issues = append(issues, Issue{Component: "dependencies", Path: id, Message: "required plugin is disabled"})
		default:
			if err := checkVersion(target.Version, dependency.Version); err != nil {
				issues = append(issues, Issue{Component: "dependencies", Path: id, Message: err.Error()})
			} else if nested := installationIssues(target, selected, visiting); len(nested) != 0 {
				for _, issue := range nested {
					issue.Message = id + " requires: " + issue.Message
					issues = append(issues, issue)
				}
			}
		}
	}
	return issues
}

// DependencyIssues exposes why enabled plugins are withheld from Bundles.
func (m *Manager) DependencyIssues() []Issue {
	m.mu.RLock()
	selected := selectInstallations(m.data.Installations)
	m.mu.RUnlock()
	var issues []Issue
	for id, installation := range selected {
		for _, issue := range installationIssues(installation, selected, map[string]bool{}) {
			issue.Message = id + ": " + issue.Message
			issues = append(issues, issue)
		}
	}
	slices.SortFunc(issues, func(a, b Issue) int { return strings.Compare(a.Message+a.Path, b.Message+b.Path) })
	return issues
}

func dependencyState(data registry) map[string]bool {
	issues := map[string]bool{}
	selected := selectInstallations(data.Installations)
	for id, installation := range selected {
		for _, issue := range installationIssues(installation, selected, map[string]bool{}) {
			issues[id+" requires "+issue.Path+": "+issue.Message] = true
		}
	}
	return issues
}

func validateDependencyTransition(old, next registry) error {
	if err := validateNamespaceTransition(old, next); err != nil {
		return err
	}
	before, after := dependencyState(old), dependencyState(next)
	var added []string
	for issue := range after {
		if !before[issue] {
			added = append(added, issue)
		}
	}
	if len(added) == 0 {
		return nil
	}
	slices.Sort(added)
	return fmt.Errorf("plugin dependency would become unavailable: %s", strings.Join(added, "; "))
}

type semanticVersion struct {
	major, minor, patch int
	prerelease          string
}

func parseVersion(text string) (semanticVersion, error) {
	v := semanticVersion{}
	text = strings.TrimPrefix(strings.TrimSpace(text), "v")
	if build := strings.IndexByte(text, '+'); build >= 0 {
		text = text[:build]
	}
	if pre := strings.IndexByte(text, '-'); pre >= 0 {
		v.prerelease = text[pre+1:]
		text = text[:pre]
	}
	parts := strings.Split(text, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return v, fmt.Errorf("invalid semantic version %q", text)
	}
	values := []*int{&v.major, &v.minor, &v.patch}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return v, fmt.Errorf("invalid semantic version %q", text)
		}
		*values[i] = n
	}
	return v, nil
}

func compareVersion(a, b semanticVersion) int {
	for _, pair := range [][2]int{{a.major, b.major}, {a.minor, b.minor}, {a.patch, b.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if a.prerelease == b.prerelease {
		return 0
	}
	if a.prerelease == "" {
		return 1
	}
	if b.prerelease == "" {
		return -1
	}
	ap, bp := strings.Split(a.prerelease, "."), strings.Split(b.prerelease, ".")
	for i := 0; i < len(ap) && i < len(bp); i++ {
		an, ae := strconv.Atoi(ap[i])
		bn, be := strconv.Atoi(bp[i])
		switch {
		case ae == nil && be == nil:
			if an < bn {
				return -1
			}
			if an > bn {
				return 1
			}
		case ae == nil:
			return -1
		case be == nil:
			return 1
		default:
			if c := strings.Compare(ap[i], bp[i]); c != 0 {
				return c
			}
		}
	}
	if len(ap) < len(bp) {
		return -1
	}
	if len(ap) > len(bp) {
		return 1
	}
	return 0
}

func checkVersion(version, constraint string) error {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" || constraint == "*" {
		return nil
	}
	actual, err := parseVersion(version)
	if err != nil {
		return fmt.Errorf("cannot check version %q against %q: %w", version, constraint, err)
	}
	var alternatives []error
	for _, branch := range strings.Split(constraint, "||") {
		fields := strings.Fields(strings.ReplaceAll(strings.TrimSpace(branch), ",", " "))
		matches := len(fields) > 0
		if actual.prerelease != "" {
			allowed := false
			for _, token := range fields {
				wanted, err := parseVersion(strings.TrimLeft(token, "><=^~"))
				if err == nil && wanted.prerelease != "" && actual.major == wanted.major && actual.minor == wanted.minor && actual.patch == wanted.patch {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		for i := 0; i < len(fields); i++ {
			token := fields[i]
			if token == "<" || token == ">" || token == "<=" || token == ">=" || token == "=" || token == "^" || token == "~" {
				i++
				if i >= len(fields) {
					return fmt.Errorf("unsupported dependency version constraint %q", constraint)
				}
				token += fields[i]
			}
			ok, err := matchVersion(actual, token)
			if err != nil {
				alternatives = append(alternatives, err)
				matches = false
				break
			}
			matches = matches && ok
		}
		if matches {
			return nil
		}
	}
	if len(alternatives) != 0 {
		return fmt.Errorf("unsupported dependency version constraint %q: %v", constraint, alternatives[0])
	}
	return fmt.Errorf("version %s does not satisfy %s", version, constraint)
}

func checkVersions(version string, constraints []string) error {
	for _, constraint := range constraints {
		if err := checkVersion(version, constraint); err != nil {
			return err
		}
	}
	return nil
}

func matchVersion(actual semanticVersion, token string) (bool, error) {
	operator := "="
	for _, prefix := range []string{">=", "<=", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(token, prefix) {
			operator = prefix
			token = strings.TrimSpace(strings.TrimPrefix(token, prefix))
			break
		}
	}
	parts := strings.Split(token, ".")
	for i, part := range parts {
		if part == "*" || strings.EqualFold(part, "x") {
			if operator != "=" {
				return false, fmt.Errorf("wildcards cannot use comparator")
			}
			base, err := parseVersion(strings.Join(parts[:i], "."))
			if i == 0 {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			return actual.major == base.major && (i == 1 || actual.minor == base.minor), nil
		}
	}
	wanted, err := parseVersion(token)
	if err != nil {
		return false, err
	}
	cmp := compareVersion(actual, wanted)
	switch operator {
	case ">=":
		return cmp >= 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case "<":
		return cmp < 0, nil
	case "^":
		upper := semanticVersion{major: wanted.major + 1}
		if wanted.major == 0 && len(parts) > 1 {
			upper = semanticVersion{minor: wanted.minor + 1}
			if wanted.minor == 0 && len(parts) > 2 {
				upper = semanticVersion{patch: wanted.patch + 1}
			}
		}
		return cmp >= 0 && compareVersion(actual, upper) < 0, nil
	case "~":
		upper := semanticVersion{major: wanted.major, minor: wanted.minor + 1}
		if len(parts) == 1 {
			upper = semanticVersion{major: wanted.major + 1}
		}
		return cmp >= 0 && compareVersion(actual, upper) < 0, nil
	default:
		if len(parts) == 1 {
			return actual.major == wanted.major, nil
		}
		if len(parts) == 2 {
			return actual.major == wanted.major && actual.minor == wanted.minor, nil
		}
		return cmp == 0, nil
	}
}
