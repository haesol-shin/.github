package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

const replaySecret = "repo-ops-go-replay-token-c4e8a1b9d7f30526"

type replayRecording struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

type replayExpected struct {
	Exit            int      `json:"exit"`
	Findings        []string `json:"findings"`
	CheckConclusion string   `json:"check_conclusion"`
	PlanDigest      string   `json:"plan_digest"`
	DiffDigest      string   `json:"diff_digest"`
}

func TestValidatorEventReplayMatchesCommittedEvidence(t *testing.T) {
	root := filepath.Join(testRepoRoot(), "tests", "testdata", "events", "validator")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	bundles := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bundle := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(bundle, "webhook.json")); err != nil {
			continue
		}
		bundles++
		t.Run(entry.Name(), func(t *testing.T) {
			replayBundle(t, bundle)
		})
	}
	if bundles == 0 {
		t.Fatal("no validator event bundles found")
	}
}

func replayBundle(t *testing.T, bundle string) {
	t.Helper()
	var meta struct {
		PullNumber int    `json:"pull_number"`
		Base       string `json:"base"`
		Head       string `json:"head"`
	}
	readReplayJSON(t, filepath.Join(bundle, "git", "meta.json"), &meta)
	if meta.PullNumber == 0 || meta.Base == "" || meta.Head == "" {
		t.Fatalf("incomplete git metadata: %+v", meta)
	}

	remote := filepath.Join(t.TempDir(), "remote.git")
	runReplayGit(t, "init", "--bare", "--quiet", remote)
	runReplayGit(t,
		"--git-dir", remote,
		"fetch", "--quiet",
		filepath.Join(bundle, "git", "repo.bundle"),
		fmt.Sprintf("refs/pull/%d/head:refs/pull/%d/head", meta.PullNumber, meta.PullNumber),
	)

	var recordings []replayRecording
	readReplayJSON(t, filepath.Join(bundle, "http", "recordings.json"), &recordings)
	index := make(map[string]replayRecording, len(recordings))
	for _, recording := range recordings {
		index[replayKey(recording.Method, recording.URL)] = recording
	}
	var server *httptest.Server
	var unmatchedMu sync.Mutex
	var unmatched []string
	server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		key := replayKey(request.Method, request.URL.RequestURI())
		recording, ok := index[key]
		if !ok {
			unmatchedMu.Lock()
			unmatched = append(unmatched, key)
			unmatchedMu.Unlock()
			http.Error(response, "not recorded", http.StatusNotFound)
			return
		}
		body := rewriteReplayValue(recording.Body, server.URL, remote)
		payload, err := json.Marshal(body)
		if err != nil {
			t.Errorf("marshal replay response: %v", err)
			http.Error(response, "invalid recording", http.StatusInternalServerError)
			return
		}
		for name, value := range recording.Headers {
			if strings.EqualFold(name, "Link") {
				value = rewriteReplayLink(value, server.URL)
			}
			response.Header().Set(name, value)
		}
		response.Header().Set("Content-Type", "application/json")
		status := recording.Status
		if status == 0 {
			status = http.StatusOK
		}
		response.WriteHeader(status)
		_, _ = response.Write(payload)
	}))
	defer server.Close()

	var webhook any
	readReplayJSON(t, filepath.Join(bundle, "webhook.json"), &webhook)
	webhook = rewriteReplayValue(webhook, server.URL, remote)
	eventPath := filepath.Join(t.TempDir(), "event.json")
	writeReplayJSON(t, eventPath, webhook)

	expected := make(map[string]replayExpected)
	readReplayJSON(t, filepath.Join(bundle, "expected.json"), &expected)
	for _, check := range []string{"all", "contract", "merge-approval"} {
		gold, ok := expected[check]
		if !ok {
			t.Fatalf("missing expected row for %s", check)
		}
		t.Run(check, func(t *testing.T) {
			digestPath := filepath.Join(t.TempDir(), "digests.json")
			getenv := func(key string) string {
				switch key {
				case "GITHUB_TOKEN":
					return replaySecret
				case "GITHUB_API_URL":
					return server.URL
				case "GITHUB_REPOSITORY":
					return "haesol-shin/.github"
				case contractRootEnv:
					return testRepoRoot()
				default:
					return ""
				}
			}
			var stdout, stderr bytes.Buffer
			code := run([]string{
				"--event", eventPath,
				"--check", check,
				"--digest-out", digestPath,
			}, &stdout, &stderr, getenv, nil)
			if strings.Contains(stdout.String()+stderr.String(), replaySecret) {
				t.Fatal("injected secret leaked")
			}
			if code != gold.Exit {
				t.Fatalf("exit = %d, want %d; stdout %q stderr %q", code, gold.Exit, stdout.String(), stderr.String())
			}
			if got := parseAnnotations(stdout.String()); !slices.Equal(got, gold.Findings) {
				t.Fatalf("findings = %v, want %v", got, gold.Findings)
			}
			conclusion := "success"
			if code != 0 {
				conclusion = "failure"
			}
			if conclusion != gold.CheckConclusion {
				t.Fatalf("conclusion = %s, want %s", conclusion, gold.CheckConclusion)
			}
			var digests struct {
				Plan string `json:"plan_digest"`
				Diff string `json:"diff_digest"`
			}
			readReplayJSON(t, digestPath, &digests)
			if digests.Plan != gold.PlanDigest || digests.Diff != gold.DiffDigest {
				t.Fatalf("digests = %+v, want plan %s diff %s", digests, gold.PlanDigest, gold.DiffDigest)
			}
		})
	}
	unmatchedMu.Lock()
	defer unmatchedMu.Unlock()
	if len(unmatched) != 0 {
		t.Fatalf("unmatched HTTP requests: %v", unmatched)
	}
}

func replayKey(method, rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return method + " " + rawURL
	}
	query := parsed.Query()
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(url.Values, len(query))
	for _, key := range keys {
		values := append([]string(nil), query[key]...)
		sort.Strings(values)
		ordered[key] = values
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if encoded := ordered.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return strings.ToUpper(method) + " " + path
}

func rewriteReplayValue(value any, origin, cloneURL string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = rewriteReplayValue(child, origin, cloneURL)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = rewriteReplayValue(child, origin, cloneURL)
		}
		return out
	case string:
		if typed == "https://github.com/haesol-shin/.github.git" ||
			typed == "https://github.com/haesol-shin/.github" ||
			typed == "git://github.com/haesol-shin/.github.git" ||
			(strings.HasSuffix(typed, ".git") && strings.Contains(typed, "github.com")) {
			return cloneURL
		}
		for _, marker := range []string{"https://api.github.com", "http://api.github.com"} {
			if strings.HasPrefix(typed, marker) {
				return origin + strings.TrimPrefix(typed, marker)
			}
		}
		return typed
	default:
		return value
	}
}

func rewriteReplayLink(value, origin string) string {
	value = strings.ReplaceAll(value, "{api}", origin)
	value = strings.ReplaceAll(value, "https://api.github.com", origin)
	return strings.ReplaceAll(value, "http://api.github.com", origin)
}

func readReplayJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func writeReplayJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runReplayGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}
