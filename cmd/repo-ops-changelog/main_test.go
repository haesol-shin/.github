package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFoldsFragments(t *testing.T) {
	root := t.TempDir()
	fragments := filepath.Join(root, "fragments")
	if err := os.Mkdir(fragments, 0o755); err != nil {
		t.Fatal(err)
	}
	fragment := filepath.Join(fragments, "5-validator.md")
	if err := os.WriteFile(fragment, []byte("## Changed\n- Use the Go validator.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changelog := filepath.Join(root, "CHANGELOG.md")
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"--root", fragments,
		"--changelog", changelog,
		"--version", "v0.1.0",
		"--date", "2026-09-21",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "folded v0.1.0 from 1 fragment(s)") {
		t.Fatalf("stdout %q", stdout.String())
	}
	if _, err := os.Stat(fragment); !os.IsNotExist(err) {
		t.Fatalf("fragment still exists: %v", err)
	}
}

func TestRunRequiresVersionAndDate(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: repo-ops-changelog") {
		t.Fatalf("stderr %q", stderr.String())
	}
}
