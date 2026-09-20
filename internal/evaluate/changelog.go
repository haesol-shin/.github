package evaluate

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const changelogRecord = "repo-ops.changelog.v1"

var (
	fragmentFilenamePattern = regexp.MustCompile(`^(?P<issue>[1-9][0-9]*|direct)-(?P<slug>[a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)
	fragmentSectionPattern  = regexp.MustCompile(`^## (?P<section>Added|Changed|Deprecated|Removed|Fixed|Security)$`)
)

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

func fragmentNameValid(filename, root string) bool {
	if !fragmentPath(filename, root) {
		return false
	}
	parts := strings.Split(filename, "/")
	return fragmentFilenamePattern.MatchString(parts[len(parts)-1])
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

	var changedFragments, deletedFragments, directChangelog, releaseChangelog []map[string]any
	for _, entry := range files {
		filename := asString(entry["filename"])
		status := strings.ToLower(asString(entry["status"]))
		if fragmentPath(filename, root) {
			if status == "added" || status == "modified" {
				changedFragments = append(changedFragments, entry)
			}
			if status == "removed" {
				deletedFragments = append(deletedFragments, entry)
			}
		}
		if normalizeSlash(filename) == "CHANGELOG.md" {
			directChangelog = append(directChangelog, entry)
			if status == "added" || status == "modified" {
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

	owned := append(append([]map[string]any{}, changedFragments...), deletedFragments...)
	for _, entry := range owned {
		filename := asString(entry["filename"])
		parts := strings.Split(filename, "/")
		name := parts[len(parts)-1]
		filenameMatch := fragmentFilenamePattern.FindStringSubmatch(name)
		if kind != "release" {
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
		} else if !fragmentNameValid(filename, root) {
			errors = append(errors, "release fragment consumption includes an invalid fragment filename")
		}
	}

	for _, entry := range changedFragments {
		content, ok := fragmentContent(state, entry)
		if !ok {
			errors = append(errors, "fragment content is unavailable for "+asString(entry["filename"]))
			continue
		}
		if err := parseFragment(content, asString(entry["filename"])); err != "" {
			errors = append(errors, err)
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

func parseFragment(text, filename string) string {
	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	empty := true
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			empty = false
			break
		}
	}
	if len(lines) == 0 || empty {
		return filename + ": fragment is empty"
	}
	sections := map[string][]string{}
	var order []string
	current := ""
	for i, line := range lines {
		number := i + 1
		if strings.TrimSpace(line) == "" {
			continue
		}
		if heading := fragmentSectionPattern.FindStringSubmatch(line); heading != nil && fragmentSectionPattern.MatchString(line) {
			current = heading[fragmentSectionPattern.SubexpIndex("section")]
			if _, exists := sections[current]; exists {
				return fmt.Sprintf("%s:%d: duplicate %s heading", filename, number, current)
			}
			sections[current] = []string{}
			order = append(order, current)
			continue
		}
		if current == "" {
			return fmt.Sprintf("%s:%d: content must follow an allowed section heading", filename, number)
		}
		if !strings.HasPrefix(line, "- ") || strings.TrimSpace(line[2:]) == "" {
			return fmt.Sprintf("%s:%d: each fragment entry must be a non-empty `- ` bullet", filename, number)
		}
		sections[current] = append(sections[current], rstrip(line))
	}
	if len(sections) == 0 {
		return filename + ": fragment has no allowed section heading"
	}
	var emptySections []string
	for _, section := range order {
		if len(sections[section]) == 0 {
			emptySections = append(emptySections, section)
		}
	}
	if len(emptySections) > 0 {
		return filename + ": section(s) have no non-empty bullet: " + strings.Join(emptySections, ", ")
	}
	return ""
}

func rstrip(s string) string {
	return strings.TrimRightFunc(s, unicode.IsSpace)
}
