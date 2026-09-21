package changelog

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var (
	sectionPattern        = regexp.MustCompile(`^## (Added|Changed|Deprecated|Removed|Fixed|Security)$`)
	versionPattern        = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	versionHeadingPattern = regexp.MustCompile(`^## \[?(v[0-9]+\.[0-9]+\.[0-9]+)\]?(?: - .*)?$`)
	fragmentNamePattern   = regexp.MustCompile(`^(?:[1-9][0-9]*|direct)-[a-z0-9]+(?:-[a-z0-9]+)*\.md$`)
)

var Sections = []string{"Added", "Changed", "Deprecated", "Removed", "Fixed", "Security"}

type Fragment struct {
	Path     string
	Sections map[string][]string
}

func ParseFragment(text, filename string) (map[string][]string, error) {
	lines := normalizedLines(text)
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 || allBlank(lines) {
		return nil, fmt.Errorf("%s: fragment is empty", filename)
	}

	sections := make(map[string][]string)
	var order []string
	current := ""
	for index, line := range lines {
		number := index + 1
		if strings.TrimSpace(line) == "" {
			continue
		}
		match := sectionPattern.FindStringSubmatch(line)
		if match != nil {
			current = match[1]
			if _, exists := sections[current]; exists {
				return nil, fmt.Errorf("%s:%d: duplicate %s heading", filename, number, current)
			}
			sections[current] = nil
			order = append(order, current)
			continue
		}
		if current == "" {
			return nil, fmt.Errorf("%s:%d: content must follow an allowed section heading", filename, number)
		}
		if !strings.HasPrefix(line, "- ") || strings.TrimSpace(line[2:]) == "" {
			return nil, fmt.Errorf("%s:%d: each fragment entry must be a non-empty `- ` bullet", filename, number)
		}
		sections[current] = append(sections[current], strings.TrimRightFunc(line, unicode.IsSpace))
	}
	if len(sections) == 0 {
		return nil, fmt.Errorf("%s: fragment has no allowed section heading", filename)
	}
	var empty []string
	for _, section := range order {
		if len(sections[section]) == 0 {
			empty = append(empty, section)
		}
	}
	if len(empty) > 0 {
		return nil, fmt.Errorf("%s: section(s) have no non-empty bullet: %s", filename, strings.Join(empty, ", "))
	}
	return sections, nil
}

func ReadFragments(root string) ([]Fragment, error) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("fragment root does not exist: %s", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".md" && entry.Name() != "README.md" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("fragment root contains no fragments: %s", root)
	}

	fragments := make([]Fragment, 0, len(names))
	for _, name := range names {
		if !fragmentNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%s: invalid fragment filename", name)
		}
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		sections, err := ParseFragment(string(data), name)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, Fragment{Path: path, Sections: sections})
	}
	return fragments, nil
}

func RenderRelease(fragments []Fragment, version, date string) string {
	grouped := make(map[string][]string)
	for _, fragment := range fragments {
		for _, section := range Sections {
			grouped[section] = append(grouped[section], fragment.Sections[section]...)
		}
	}
	lines := []string{fmt.Sprintf("## [%s] - %s", version, date), ""}
	for _, section := range Sections {
		bullets := grouped[section]
		if len(bullets) == 0 {
			continue
		}
		lines = append(lines, "### "+section)
		lines = append(lines, bullets...)
		lines = append(lines, "")
	}
	return strings.TrimRightFunc(strings.Join(lines, "\n"), unicode.IsSpace) + "\n"
}

func Fold(root, changelogPath, version, date string) ([]string, error) {
	if !versionPattern.MatchString(version) {
		return nil, fmt.Errorf("version must match vX.Y.Z")
	}
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil || parsedDate.Format("2006-01-02") != date {
		return nil, fmt.Errorf("date must be an ISO-8601 calendar date")
	}
	fragments, err := ReadFragments(root)
	if err != nil {
		return nil, err
	}
	existing, err := os.ReadFile(changelogPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if existingVersion(string(existing), version) {
		return nil, fmt.Errorf("changelog already contains %s", version)
	}
	if err := assertClean(changelogPath); err != nil {
		return nil, err
	}

	prefix := string(existing)
	if prefix != "" && !strings.HasSuffix(prefix, "\n") {
		prefix += "\n"
	}
	if prefix != "" && !strings.HasSuffix(prefix, "\n\n") {
		prefix += "\n"
	}
	if err := atomicWrite(changelogPath, prefix+RenderRelease(fragments, version, date)); err != nil {
		return nil, err
	}

	consumed := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		if err := os.Remove(fragment.Path); err != nil {
			return nil, err
		}
		consumed = append(consumed, fragment.Path)
	}
	return consumed, nil
}

func normalizedLines(text string) []string {
	return strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n")
}

func allBlank(lines []string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			return false
		}
	}
	return true
}

func existingVersion(text, version string) bool {
	scanner := bufio.NewScanner(strings.NewReader(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")))
	for scanner.Scan() {
		match := versionHeadingPattern.FindStringSubmatch(scanner.Text())
		if match != nil && match[1] == version {
			return true
		}
	}
	return false
}

func assertClean(path string) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	dir := filepath.Dir(path)
	rootBytes, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil
	}
	root := strings.TrimSpace(string(rootBytes))
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	relative, err := filepath.Rel(root, absolutePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil
	}
	for _, args := range [][]string{{"diff", "--quiet", "--", relative}, {"diff", "--cached", "--quiet", "--", relative}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("changelog is dirty: %s", filepath.ToSlash(relative))
		}
	}
	return nil
}

func atomicWrite(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
