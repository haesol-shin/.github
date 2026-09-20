package main

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUsageRequiresExactlyOneSource(t *testing.T) {
	t.Parallel()
	cases := [][]string{
		nil,
		{"--check", "all"},
		{"--fixture", "fixtures/valid-high.json", "--event", "event.json"},
	}
	for _, args := range cases {
		stdout, stderr, code := capture(args)
		if code != 2 {
			t.Fatalf("args %v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
		if strings.Contains(stdout, annotationPrefix) {
			t.Fatalf("usage error leaked annotations: %q", stdout)
		}
		if !strings.Contains(stderr, exactlyOneSourceRequired) {
			t.Fatalf("args %v: stderr %q", args, stderr)
		}
	}
}

func TestStage1EventIsUnsupported(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := capture([]string{"--event", "does-not-exist.json"})
	if code != 2 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Contains(stdout, annotationPrefix) {
		t.Fatalf("event path leaked annotations: %q", stdout)
	}
	if !strings.Contains(stderr, stage1EventUnsupported) {
		t.Fatalf("stderr %q", stderr)
	}
	if strings.Contains(stderr, "GITHUB_TOKEN") {
		t.Fatalf("event rejection mentioned token name: %q", stderr)
	}
}

func TestInvalidCheckExitsTwo(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := capture([]string{"--fixture", "fixtures/valid-high.json", "--check", "quality"})
	if code != 2 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Contains(stdout, annotationPrefix) {
		t.Fatalf("invalid check leaked annotations: %q", stdout)
	}
	if !strings.Contains(stderr, "invalid --check") {
		t.Fatalf("stderr %q", stderr)
	}
}

func TestValidHighFixtureSucceeds(t *testing.T) {
	path := fixturePath(t, "valid-high.json")
	stdout, stderr, code := capture([]string{"--fixture", path, "--check", "all"})
	if code != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Contains(stdout, annotationPrefix) {
		t.Fatalf("valid fixture produced warnings: %q", stdout)
	}
	if !strings.Contains(stdout, "Repository policy / all: contract satisfied") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestMergeApprovalBlockedWhenContractFails(t *testing.T) {
	path := fixturePath(t, "invalid-required-missing.json")
	stdout, stderr, code := capture([]string{"--fixture", path, "--check", "merge-approval"})
	if code != 1 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	findings := parseAnnotations(stdout)
	if len(findings) == 0 || findings[0] != mergeApprovalBlocked {
		t.Fatalf("findings %q", findings)
	}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "fixtures", name)
}

func parseAnnotations(stdout string) []string {
	var findings []string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, annotationPrefix) {
			findings = append(findings, strings.TrimPrefix(line, annotationPrefix))
		}
	}
	return findings
}

func capture(args []string) (string, string, int) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr, func(string) string { return "" })
	return stdout.String(), stderr.String(), code
}
