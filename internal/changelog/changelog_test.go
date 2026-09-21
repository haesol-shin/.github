package changelog

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseFragmentStrictErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "empty", text: "\n", want: "note.md: fragment is empty"},
		{name: "unknown heading", text: "## Notes\n- no\n", want: "note.md:1: content must follow an allowed section heading"},
		{name: "duplicate", text: "## Added\n- one\n## Added\n- two\n", want: "note.md:3: duplicate Added heading"},
		{name: "empty section", text: "## Added\n", want: "note.md: section(s) have no non-empty bullet: Added"},
		{name: "not bullet", text: "## Added\nplain\n", want: "note.md:2: each fragment entry must be a non-empty `- ` bullet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseFragment(tc.text, "note.md")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestFoldStableDuplicateSafeAndConsumesOnlyFragments(t *testing.T) {
	root := t.TempDir()
	fragments := filepath.Join(root, "changelog.d")
	if err := os.Mkdir(fragments, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(fragments, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("3-z-last.md", "## Added\n- Z entry.\n")
	write("3-a-first.md", "## Added\n- A entry.\n")
	keep := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	changelogPath := filepath.Join(root, "CHANGELOG.md")
	consumed, err := Fold(fragments, changelogPath, "v0.1.0", "2026-09-21")
	if err != nil {
		t.Fatal(err)
	}
	wantConsumed := []string{filepath.Join(fragments, "3-a-first.md"), filepath.Join(fragments, "3-z-last.md")}
	if !reflect.DeepEqual(consumed, wantConsumed) {
		t.Fatalf("consumed = %v, want %v", consumed, wantConsumed)
	}
	data, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "## [v0.1.0] - 2026-09-21\n\n### Added\n- A entry.\n- Z entry.\n"
	if string(data) != want {
		t.Fatalf("changelog = %q, want %q", data, want)
	}
	if data, err := os.ReadFile(keep); err != nil || string(data) != "keep" {
		t.Fatalf("unrelated file = %q, %v", data, err)
	}

	write("3-again.md", "## Fixed\n- A fix.\n")
	_, err = Fold(fragments, changelogPath, "v0.1.0", "2026-09-21")
	if err == nil || !strings.Contains(err.Error(), "already contains v0.1.0") {
		t.Fatalf("duplicate version error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fragments, "3-again.md")); err != nil {
		t.Fatalf("duplicate failure consumed fragment: %v", err)
	}
}

func TestFoldRestoresChangelogAndFragmentsAfterConsumeFailure(t *testing.T) {
	root := t.TempDir()
	fragments := filepath.Join(root, "changelog.d")
	if err := os.Mkdir(fragments, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(fragments, "3-first.md")
	second := filepath.Join(fragments, "3-second.md")
	firstContent := "## Added\n- First.\n"
	secondContent := "## Fixed\n- Second.\n"
	if err := os.WriteFile(first, []byte(firstContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte(secondContent), 0o644); err != nil {
		t.Fatal(err)
	}
	changelogPath := filepath.Join(root, "CHANGELOG.md")
	existing := "## [v0.0.1] - 2026-09-20\n"
	if err := os.WriteFile(changelogPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	removals := 0
	_, err := fold(fragments, changelogPath, "v0.1.0", "2026-09-21", func(path string) error {
		removals++
		if removals == 2 {
			return errors.New("injected consume failure")
		}
		return os.Remove(path)
	})
	if err == nil || !strings.Contains(err.Error(), "injected consume failure") {
		t.Fatalf("error = %v", err)
	}
	for path, want := range map[string]string{first: firstContent, second: secondContent, changelogPath: existing} {
		data, readErr := os.ReadFile(path)
		if readErr != nil || string(data) != want {
			t.Fatalf("%s = %q, %v; want %q", path, data, readErr, want)
		}
	}
}

func TestFoldRejectsInvalidVersionAndDateBeforeMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, tc := range []struct {
		version string
		date    string
		want    string
	}{
		{version: "0.1.0", date: "2026-09-21", want: "version must match vX.Y.Z"},
		{version: "v0.1.0", date: "2026-02-30", want: "date must be an ISO-8601 calendar date"},
	} {
		_, err := Fold(filepath.Join(root, "missing"), filepath.Join(root, "CHANGELOG.md"), tc.version, tc.date)
		if err == nil || err.Error() != tc.want {
			t.Fatalf("error = %v, want %q", err, tc.want)
		}
	}
}

func TestFoldRejectsDuplicateAfterLongLine(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	fragments := filepath.Join(root, "changelog.d")
	if err := os.Mkdir(fragments, 0o755); err != nil {
		t.Fatal(err)
	}
	fragment := filepath.Join(fragments, "3-note.md")
	if err := os.WriteFile(fragment, []byte("## Added\n- Entry.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changelogPath := filepath.Join(root, "CHANGELOG.md")
	existing := strings.Repeat("x", 70*1024) + "\n## [v0.1.0] - 2026-09-20\n"
	if err := os.WriteFile(changelogPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Fold(fragments, changelogPath, "v0.1.0", "2026-09-21")
	if err == nil || !strings.Contains(err.Error(), "already contains v0.1.0") {
		t.Fatalf("duplicate version error = %v", err)
	}
	if _, err := os.Stat(fragment); err != nil {
		t.Fatalf("duplicate failure consumed fragment: %v", err)
	}
}

func TestFoldRejectsInvalidUTF8BeforeMutation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name              string
		fragment          []byte
		existingChangelog []byte
		want              string
	}{
		{
			name:     "fragment",
			fragment: append([]byte("## Added\n- Entry "), 0xff, '\n'),
			want:     "fragment is not valid UTF-8",
		},
		{
			name:              "changelog",
			fragment:          []byte("## Added\n- Entry.\n"),
			existingChangelog: []byte{0xff, '\n'},
			want:              "changelog is not valid UTF-8",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			fragments := filepath.Join(root, "changelog.d")
			if err := os.Mkdir(fragments, 0o755); err != nil {
				t.Fatal(err)
			}
			fragment := filepath.Join(fragments, "3-note.md")
			if err := os.WriteFile(fragment, tc.fragment, 0o644); err != nil {
				t.Fatal(err)
			}
			changelogPath := filepath.Join(root, "CHANGELOG.md")
			if tc.existingChangelog != nil {
				if err := os.WriteFile(changelogPath, tc.existingChangelog, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Fold(fragments, changelogPath, "v0.1.0", "2026-09-21")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if _, err := os.Stat(fragment); err != nil {
				t.Fatalf("UTF-8 failure consumed fragment: %v", err)
			}
		})
	}
}
