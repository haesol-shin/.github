package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/haesol-shin/.github/internal/collect"
	"github.com/haesol-shin/.github/internal/evaluate"
	"github.com/haesol-shin/.github/internal/fixture"
)

const (
	annotationPrefix         = "::warning title=Repository policy::"
	mergeApprovalBlocked     = "contract check did not succeed; merge approval is blocked"
	exactlyOneSourceRequired = "exactly one of --fixture or --event is required"
	contractMarker           = "contracts/v0.1.0/contract.json"
)

// liveStateFunc loads evaluator state from a GitHub event path. Production uses
// collect.BuildLiveState; tests inject a fake to avoid live GitHub and git.
type liveStateFunc func(eventPath string, getenv func(string) string) (map[string]any, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv, nil))
}

func defaultLiveState(eventPath string, getenv func(string) string) (map[string]any, error) {
	return collect.BuildLiveState(
		context.Background(),
		eventPath,
		getenv,
		collect.NewGitHubClient(getenv("GITHUB_TOKEN"), getenv("GITHUB_API_URL"), nil),
		collect.NewGitRunner(),
	)
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string, live liveStateFunc) int {
	if live == nil {
		live = defaultLiveState
	}
	fs := flag.NewFlagSet("repo-ops-validator", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: repo-ops-validator --fixture PATH | --event PATH [--check all|contract|merge-approval]\n")
	}
	fixturePath := fs.String("fixture", "", "fixture state JSON")
	eventPath := fs.String("event", "", "GitHub event JSON")
	checkName := fs.String("check", "all", "all, contract, or merge-approval")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}

	check, err := parseCheck(*checkName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	hasFixture := strings.TrimSpace(*fixturePath) != ""
	hasEvent := strings.TrimSpace(*eventPath) != ""
	if hasFixture == hasEvent {
		fmt.Fprintln(stderr, exactlyOneSourceRequired)
		return 2
	}

	hint := *fixturePath
	if hasEvent {
		hint = *eventPath
	}
	root, err := findRepoRoot(hint)
	if err != nil {
		return emitFindings(stdout, getenv, []string{err.Error()}, *checkName)
	}
	contractRoot := filepath.Join(root, "contracts", "v0.1.0")

	var state map[string]any
	if hasEvent {
		state, err = live(*eventPath, getenv)
	} else {
		state, _, err = fixture.Load(*fixturePath)
	}
	if err != nil {
		return emitFindings(stdout, getenv, []string{err.Error()}, *checkName)
	}

	var findings []string
	if check == evaluate.CheckMergeApproval {
		contractFindings := evaluate.ValidateState(state, contractRoot, evaluate.CheckContract)
		findings = evaluate.ValidateState(state, contractRoot, evaluate.CheckMergeApproval)
		if len(contractFindings) > 0 {
			findings = append([]string{mergeApprovalBlocked}, findings...)
		}
	} else {
		findings = evaluate.ValidateState(state, contractRoot, check)
	}
	return emitFindings(stdout, getenv, findings, *checkName)
}

func parseCheck(name string) (evaluate.Check, error) {
	switch name {
	case "all":
		return evaluate.CheckAll, nil
	case "contract":
		return evaluate.CheckContract, nil
	case "merge-approval":
		return evaluate.CheckMergeApproval, nil
	default:
		var zero evaluate.Check
		return zero, fmt.Errorf("invalid --check %q (want all, contract, or merge-approval)", name)
	}
}

func emitFindings(stdout io.Writer, getenv func(string) string, findings []string, check string) int {
	if len(findings) == 0 {
		fmt.Fprintf(stdout, "Repository policy / %s: contract satisfied\n", check)
		return 0
	}
	for _, finding := range findings {
		fmt.Fprintf(stdout, "%s%s\n", annotationPrefix, finding)
	}
	if summary := getenv("GITHUB_STEP_SUMMARY"); summary != "" {
		f, err := os.OpenFile(summary, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "## Repository policy / %s findings\n\n", check)
			for _, finding := range findings {
				fmt.Fprintf(f, "- %s\n", finding)
			}
			_ = f.Close()
		}
	}
	fmt.Fprintf(stdout, "Repository policy / %s: %d finding(s)\n", check, len(findings))
	return 1
}

func findRepoRoot(hint string) (string, error) {
	var starts []string
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if hint != "" {
		starts = append(starts, filepath.Dir(hint))
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	seen := map[string]struct{}{}
	for _, start := range starts {
		start = filepath.Clean(start)
		for dir := start; ; {
			if _, dup := seen[dir]; dup {
				break
			}
			seen[dir] = struct{}{}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(contractMarker))); err == nil {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("repository root with %s not found", contractMarker)
}
