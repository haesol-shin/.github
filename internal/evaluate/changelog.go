package evaluate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/haesol-shin/.github/internal/changelog"
)

const changelogRecord = "repo-ops.changelog.v1"

var fragmentFilenamePattern = regexp.MustCompile(`^(?P<issue>[1-9][0-9]*|direct)-(?P<slug>[a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)

func parseChangelogDeclaration(body, contractRoot string, orders map[string][]string) (map[string]string, []string) {
	var candidates []string
	for _, line := range unfencedLines(body) {
		stripped := strings.TrimSpace(line)
		if stripped == "" {
			continue
		}
		if fields := strings.Fields(stripped); fields[0] == changelogRecord {
			candidates = append(candidates, stripped)
		}
	}
	if len(candidates) != 1 {
		return nil, []string{"pull request body must contain exactly one repo-ops.changelog.v1 record"}
	}
	parsed, err := parseRecordLine(candidates[0], changelogRecord, orders)
	if parsed == nil || err != "" {
		return nil, []string{"malformed repo-ops.changelog.v1 record"}
	}
	schemaErrors := schemaFindings(
		parsed,
		filepath.Join(contractRoot, "changelog-declaration.schema.json"),
		"changelog declaration",
	)
	if len(schemaErrors) > 0 {
		return nil, schemaErrors
	}
	return map[string]string{
		"kind":  asString(parsed["kind"]),
		"value": asString(parsed["value"]),
	}, nil
}

func stateFiles(state map[string]any) []map[string]any {
	pullRequest := asMap(state["pull_request"])
	files := state["files"]
	if _, ok := state["files"]; !ok {
		if pullRequest != nil {
			files = pullRequest["files"]
		}
	}
	var out []map[string]any
	for _, entry := range asEntries(files) {
		if _, ok := entry["filename"].(string); ok {
			out = append(out, entry)
		}
	}
	return out
}

func fragmentPath(filename, root string) bool {
	normalized := normalizeSlash(filename)
	prefix := strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/") + "/"
	if !strings.HasPrefix(normalized, prefix) {
		return false
	}
	relative := normalized[len(prefix):]
	return !strings.Contains(relative, "/") && strings.HasSuffix(relative, ".md") && relative != "README.md"
}

func supportedChangelogStatus(status string) bool {
	switch status {
	case "added", "modified", "renamed", "copied", "removed":
		return true
	default:
		return false
	}
}

func fragmentContent(state map[string]any, entry map[string]any) (string, bool) {
	filename := asString(entry["filename"])
	for _, key := range []string{"file_contents", "fragment_contents"} {
		contents := asMap(state[key])
		if contents != nil {
			if text, ok := contents[filename].(string); ok {
				return text, true
			}
		}
	}
	if text, ok := entry["content"].(string); ok {
		return text, true
	}
	return "", false
}

func validateChangelogState(state map[string]any, policy map[string]any, issue any, direct bool, declaration map[string]string) []string {
	if declaration == nil {
		return nil
	}
	var errors []string
	files := stateFiles(state)
	changelogPolicy := asMap(policy["changelog"])
	mode := ""
	root := ""
	if changelogPolicy != nil {
		mode = asString(changelogPolicy["mode"])
		root = asString(changelogPolicy["root"])
	}
	if mode != "fragments" {
		root = ""
	}
	kind := declaration["kind"]
	if (kind == "required" || kind == "release") && mode != "fragments" {
		return []string{"changelog fragments are required but repository policy does not enable fragments"}
	}
	release := asMap(policy["release"])
	enabled := false
	if release != nil {
		if b, ok := asBool(release["enabled"]); ok {
			enabled = b
		}
	}
	if kind == "release" && !enabled {
		errors = append(errors, "release changelog declarations require release folding to be enabled")
	}
	if root == "" {
		root = "changelog.d"
	}

	const unsupportedStatus = "changelog file status must be added, modified, renamed, copied, or removed"
	var changedFragments, deletedFragments, directChangelog, releaseChangelog []map[string]any
	for _, entry := range files {
		filename := asString(entry["filename"])
		status := strings.ToLower(asString(entry["status"]))
		previous := asString(entry["previous_filename"])
		changelogRelated := fragmentPath(filename, root) || normalizeSlash(filename) == "CHANGELOG.md" ||
			fragmentPath(previous, root) || normalizeSlash(previous) == "CHANGELOG.md"
		if changelogRelated && !supportedChangelogStatus(status) {
			errors = append(errors, unsupportedStatus)
			continue
		}
		if fragmentPath(filename, root) {
			switch status {
			case "added", "modified", "renamed", "copied":
				changedFragments = append(changedFragments, entry)
			case "removed":
				deletedFragments = append(deletedFragments, entry)
			}
		}
		if status == "renamed" {
			if previous != "" && fragmentPath(previous, root) {
				deletedFragments = append(deletedFragments, map[string]any{
					"filename": previous,
					"status":   "removed",
				})
			}
			if normalizeSlash(previous) == "CHANGELOG.md" {
				directChangelog = append(directChangelog, entry)
			}
		}
		if normalizeSlash(filename) == "CHANGELOG.md" {
			directChangelog = append(directChangelog, entry)
			switch status {
			case "added", "modified", "renamed", "copied":
				releaseChangelog = append(releaseChangelog, entry)
			}
		}
	}

	if kind != "release" && len(directChangelog) > 0 {
		errors = append(errors, "ordinary pull requests must not edit CHANGELOG.md")
	}
	if kind != "release" && len(deletedFragments) > 0 {
		errors = append(errors, "fragment deletion is reserved for release changelog declarations")
	}
	if kind == "required" && len(changedFragments) == 0 {
		errors = append(errors, "required changelog declarations need an added or modified fragment")
	}
	if kind == "release" {
		if len(releaseChangelog) == 0 {
			errors = append(errors, "release changelog declarations must update CHANGELOG.md")
		}
		if len(deletedFragments) == 0 {
			errors = append(errors, "release changelog declarations must consume at least one fragment")
		}
		if len(changedFragments) > 0 {
			errors = append(errors, "release changelog declarations must consume, not modify, fragments")
		}
	}

	for _, entry := range changedFragments {
		filename := asString(entry["filename"])
		parts := strings.Split(filename, "/")
		name := parts[len(parts)-1]
		filenameMatch := fragmentFilenamePattern.FindStringSubmatch(name)
		if filenameMatch == nil {
			errors = append(errors, "fragment filename must use <issue>-<slug>.md or direct-<slug>.md")
		} else if direct {
			if filenameMatch[fragmentFilenamePattern.SubexpIndex("issue")] != "direct" {
				errors = append(errors, "direct low-risk fragments must use direct-<slug>.md")
			}
		} else if issue == nil {
			errors = append(errors, "issue-backed fragments require a linked issue")
		} else if filenameMatch[fragmentFilenamePattern.SubexpIndex("issue")] != issueString(issue) {
			errors = append(errors, "issue-backed fragment filename must begin with the linked issue number")
		}
	}

	for _, entry := range changedFragments {
		content, ok := fragmentContent(state, entry)
		if !ok {
			errors = append(errors, "fragment content is unavailable for "+asString(entry["filename"]))
			continue
		}
		if _, err := changelog.ParseFragment(content, asString(entry["filename"])); err != nil {
			errors = append(errors, err.Error())
		}
	}
	return errors
}

func issueString(v any) string {
	if n, ok := asInt(v); ok {
		return strconv.Itoa(n)
	}
	return fmt.Sprint(v)
}
