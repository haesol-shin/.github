package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/haesol-shin/.github/internal/fixture"
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

func TestHelpOmitsUnsupported(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := capture([]string{"-h"})
	if code != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	combined := stdout + stderr
	if strings.Contains(strings.ToLower(combined), "unsupported") {
		t.Fatalf("help still mentions unsupported: %q", combined)
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

func TestInvalidCheckWithEventSkipsCollector(t *testing.T) {
	t.Parallel()
	stdout, stderr, code := captureLive([]string{"--event", "event.json", "--check", "quality"}, func(string, func(string) string) (map[string]any, error) {
		t.Fatal("collector ran before invalid --check was rejected")
		return nil, nil
	})
	if code != 2 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Contains(stdout, annotationPrefix) {
		t.Fatalf("invalid check leaked annotations: %q", stdout)
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

func TestEventCollectorErrorBecomesFinding(t *testing.T) {
	t.Parallel()
	const secret = "ghs_cli-test-token-value"
	const message = "GITHUB_TOKEN is required for live validation"
	stdout, stderr, code := captureLiveEnv(
		[]string{"--event", "event.json"},
		func(key string) string {
			if key == "GITHUB_TOKEN" {
				return secret
			}
			return ""
		},
		func(eventPath string, getenv func(string) string) (map[string]any, error) {
			if eventPath != "event.json" {
				return nil, fmt.Errorf("unexpected event path %q", eventPath)
			}
			if getenv("GITHUB_TOKEN") != secret {
				return nil, fmt.Errorf("getenv not forwarded")
			}
			return nil, fmt.Errorf("%s", message)
		},
	)
	if code != 1 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Contains(stdout+stderr, "unsupported") {
		t.Fatalf("event path still reports unsupported: stdout %q stderr %q", stdout, stderr)
	}
	if strings.Contains(stdout, secret) || strings.Contains(stderr, secret) {
		t.Fatalf("token value leaked: stdout %q stderr %q", stdout, stderr)
	}
	findings := parseAnnotations(stdout)
	if len(findings) != 1 || findings[0] != message {
		t.Fatalf("findings %q", findings)
	}
	if !strings.Contains(stdout, "Repository policy / all: 1 finding(s)") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestEventLoadsCollectorStateAndEvaluates(t *testing.T) {
	path := fixturePath(t, "valid-high.json")
	state, _, err := fixture.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := captureLive([]string{"--event", "event.json", "--check", "all"}, func(eventPath string, _ func(string) string) (map[string]any, error) {
		if eventPath != "event.json" {
			return nil, fmt.Errorf("unexpected event path %q", eventPath)
		}
		return state, nil
	})
	if code != 0 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if strings.Contains(stdout, annotationPrefix) {
		t.Fatalf("valid collected state produced warnings: %q", stdout)
	}
	if !strings.Contains(stdout, "Repository policy / all: contract satisfied") {
		t.Fatalf("stdout %q", stdout)
	}
}

func TestEventMergeApprovalBlockedWhenContractFails(t *testing.T) {
	path := fixturePath(t, "invalid-required-missing.json")
	state, _, err := fixture.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := captureLive([]string{"--event", "event.json", "--check", "merge-approval"}, func(string, func(string) string) (map[string]any, error) {
		return state, nil
	})
	if code != 1 {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	findings := parseAnnotations(stdout)
	if len(findings) == 0 || findings[0] != mergeApprovalBlocked {
		t.Fatalf("findings %q", findings)
	}
}

func TestEventRootRequiresTrustedLocation(t *testing.T) {
	t.Parallel()
	if _, err := findRepoRoot("event.json", true, func(string) string { return "" }); err == nil {
		t.Fatal("event root trusted the ambient working directory")
	}
	root, err := findRepoRoot("event.json", true, func(key string) string {
		if key == contractRootEnv {
			return testRepoRoot()
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if root != testRepoRoot() {
		t.Fatalf("root = %q, want %q", root, testRepoRoot())
	}
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(testRepoRoot(), "fixtures", name)
}

func testRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
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
	return captureLive(args, func(string, func(string) string) (map[string]any, error) {
		panic("live collector should not run in fixture or usage tests")
	})
}

func captureLive(args []string, live liveStateFunc) (string, string, int) {
	return captureLiveEnv(args, func(string) string { return "" }, live)
}

func captureLiveEnv(args []string, getenv func(string) string, live liveStateFunc) (string, string, int) {
	var stdout, stderr bytes.Buffer
	trustedEnv := func(key string) string {
		if key == contractRootEnv {
			return testRepoRoot()
		}
		return getenv(key)
	}
	code := run(args, &stdout, &stderr, trustedEnv, live)
	return stdout.String(), stderr.String(), code
}
