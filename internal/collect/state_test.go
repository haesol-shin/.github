package collect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/haesol-shin/.github/internal/canonical"
)

const (
	baseSHA  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	headSHA  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	planSHA  = "dddddddddddddddddddddddddddddddddddddddd"
	mergeSHA = "cccccccccccccccccccccccccccccccccccccccc"
)

func TestBuildLiveStateOwnershipAndSelection(t *testing.T) {
	basePolicy := "contract: repo-ops/v0.1.0\nprofile: operations\nquality:\n  check: quality\n  commands: [test]\nreview:\n  shared_identity: true\nchangelog:\n  mode: fragments\n  root: changelog.d\nrelease:\n  enabled: true\n"
	headPolicy := "contract: repo-ops/v0.1.0\nprofile: adversarial\n"
	fragment := "## Added\n- Head fragment.\n"
	headPlan := "head plan text\n"
	approvalPlan := "head plan text\n"
	planDigest, err := canonical.PlanDigest(headPlan)
	if err != nil {
		t.Fatal(err)
	}
	intent := "sha256:" + strings.Repeat("e", 64)
	approvalBody := planApprovalBody("high", intent, planDigest, planSHA)

	var hits []string
	permHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.Method+" "+r.URL.RequestURI())
		if got := r.Header.Get("Authorization"); got != "Bearer "+testToken {
			t.Errorf("auth %q", got)
		}
		if strings.Contains(r.URL.RawQuery, testToken) || strings.Contains(r.URL.Path, testToken) {
			t.Fatal("token in URL")
		}
		switch r.URL.RequestURI() {
		case "/repos/o/r/contents/.github/repo-policy.yml?ref=" + baseSHA:
			writeContent(w, basePolicy)
		case "/repos/o/r/contents/.github/repo-policy.yml?ref=" + headSHA:
			writeContent(w, headPolicy)
		case "/repos/o/r/pulls/9/files?per_page=100":
			w.Header().Set("Link", `<http://`+r.Host+`/repos/o/r/pulls/9/files?per_page=100&page=2>; rel="next"`)
			io.WriteString(w, `[{"filename":"changelog.d/9-head.md","status":"added","additions":1,"deletions":0,"changes":1}]`)
		case "/repos/o/r/pulls/9/files?per_page=100&page=2":
			io.WriteString(w, `[{"filename":"README.md","status":"modified","additions":1,"deletions":0,"changes":1}]`)
		case "/repos/o/r/contents/changelog.d/9-head.md?ref=" + headSHA:
			writeContent(w, fragment)
		case "/repos/o/r/contents/changelog.d/9-head.md?ref=" + baseSHA:
			http.Error(w, "base should not load fragment", http.StatusNotFound)
		case fmt.Sprintf("/repos/o/r/compare/%s...%s", baseSHA, headSHA):
			fmt.Fprintf(w, `{"merge_base_commit":{"sha":%q},"status":"ahead"}`, mergeSHA)
		case "/repos/o/r/issues/9/comments?per_page=100":
			io.WriteString(w, `[
				{"body":`+jsonString(approvalBody)+`,"user":{"login":"maintainer"},"author_association":"OWNER"},
				{"body":"noise","user":{"login":"maintainer"},"author_association":"OWNER"}
			]`)
		case "/repos/o/r/pulls/9/reviews?per_page=100":
			io.WriteString(w, `[
				{"body":"","state":"COMMENTED","user":{"login":"maintainer"},"author_association":"OWNER"},
				{"body":"stale","state":"DISMISSED","user":{"login":"maintainer"},"author_association":"OWNER"},
				{"body":"looks good","state":"APPROVED","user":{"login":"reviewer"},"author_association":"MEMBER"}
			]`)
		case "/repos/o/r/issues/42":
			io.WriteString(w, `{"body":"## Problem\n\nNeed it.\n"}`)
		case "/repos/o/r/issues/42/comments?per_page=100":
			io.WriteString(w, `[{"body":"intent comment","user":{"login":"maintainer"},"author_association":"OWNER"}]`)
		case "/repos/o/r/contents/.ops/plans/42-policy-validator.md?ref=" + headSHA:
			writeContent(w, headPlan)
		case "/repos/o/r/contents/.ops/plans/42-policy-validator.md?ref=" + planSHA:
			writeContent(w, approvalPlan)
		case fmt.Sprintf("/repos/o/r/compare/%s...%s", planSHA, headSHA):
			io.WriteString(w, `{"status":"ahead"}`)
		case "/repos/o/r/commits/" + headSHA + "/check-runs?per_page=100":
			io.WriteString(w, `{"check_runs":[
				{"name":"quality","conclusion":"failure"},
				{"name":"CI / quality","conclusion":"success"},
				{"name":"quality","conclusion":"success"}
			]}`)
		case "/repos/o/r/collaborators/maintainer/permission":
			permHits++
			io.WriteString(w, `{"permission":"admin"}`)
		case "/repos/o/r/collaborators/reviewer/permission":
			http.Error(w, "missing", http.StatusNotFound)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	event := map[string]any{
		"repository": map[string]any{"full_name": "o/r"},
		"pull_request": map[string]any{
			"number": 9,
			"body":   "Risk: high — enforcement\n\nFixes #42\nPlan: .ops/plans/42-policy-validator.md\n",
			"user":   map[string]any{"login": "author"},
			"base":   map[string]any{"sha": baseSHA, "repo": map[string]any{"clone_url": "https://github.com/o/r.git"}},
			"head":   map[string]any{"sha": headSHA},
		},
	}
	path := writeEvent(t, event)
	git := stubGit{base: mergeSHA, digest: "sha256:" + strings.Repeat("c", 64)}
	client := NewGitHubClient(testToken, server.URL, server.Client())
	state, err := BuildLiveState(context.Background(), path, testEnv(server.URL), client, git)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprintf("%v", state), testToken) {
		t.Fatal("token leaked into state")
	}

	policy := asMap(state["policy"])
	if policy["profile"] != "operations" {
		t.Fatalf("base policy not used: %#v", policy)
	}
	contents := asMap(state["file_contents"])
	if contents["changelog.d/9-head.md"] != fragment {
		t.Fatalf("fragment %#v", contents)
	}
	if _, ok := contents["README.md"]; ok {
		t.Fatal("non-fragment should not be loaded")
	}
	pr := asMap(state["pull_request"])
	if pr["base"] != baseSHA || pr["head"] != headSHA || pr["merge_base"] != mergeSHA {
		t.Fatalf("shas %#v", pr)
	}
	if pr["diff_digest"] != git.digest {
		t.Fatalf("digest %#v", pr["diff_digest"])
	}
	files, _ := state["files"].([]any)
	if len(files) != 2 {
		t.Fatalf("paginated files %#v", files)
	}
	if state["issue"] != 42 {
		t.Fatalf("issue %#v", state["issue"])
	}
	if state["issue_body"] != "## Problem\n\nNeed it.\n" {
		t.Fatalf("issue_body %#v", state["issue_body"])
	}
	intents, _ := state["intent_comments"].([]any)
	if len(intents) != 1 {
		t.Fatalf("intent %#v", intents)
	}
	if asMap(intents[0])["permission"] != "admin" {
		t.Fatalf("intent permission %#v", intents[0])
	}
	comments, _ := state["comments"].([]any)
	if len(comments) != 2 {
		t.Fatalf("comments %#v", comments)
	}
	if asMap(comments[0])["permission"] != "admin" {
		t.Fatalf("approval permission %#v", comments[0])
	}
	if asMap(comments[1])["permission"] != nil {
		t.Fatalf("noise comment should not lookup permission: %#v", comments[1])
	}
	reviews, _ := state["reviews"].([]any)
	if len(reviews) != 1 || asMap(reviews[0])["body"] != "looks good" {
		t.Fatalf("reviews %#v", reviews)
	}
	if asMap(reviews[0])["permission"] != nil {
		t.Fatalf("review without marker should not require permission: %#v", reviews[0])
	}
	if state["plan_digest"] != planDigest {
		t.Fatalf("plan digest %v want %s", state["plan_digest"], planDigest)
	}
	commits, _ := state["plan_commits"].([]any)
	if len(commits) != 1 || commits[0] != planSHA {
		t.Fatalf("plan_commits %#v", commits)
	}
	quality := asMap(state["quality"])
	if quality["name"] != "quality" || quality["conclusion"] != "success" {
		t.Fatalf("quality last-match %#v", quality)
	}
	if permHits != 1 {
		t.Fatalf("permission cache hits %d", permHits)
	}
	joined := strings.Join(hits, "\n")
	if !strings.Contains(joined, "repo-policy.yml?ref="+baseSHA) {
		t.Fatalf("missing base policy fetch:\n%s", joined)
	}
	if strings.Contains(joined, "repo-policy.yml?ref="+headSHA) {
		t.Fatalf("loaded head policy:\n%s", joined)
	}
	if !strings.Contains(joined, "9-head.md?ref="+headSHA) {
		t.Fatalf("missing head fragment:\n%s", joined)
	}
	if !strings.Contains(joined, "check-runs?per_page=100") {
		t.Fatalf("missing check-runs query:\n%s", joined)
	}
}

func TestBuildLiveStateIssuePullRequestURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pr":
			io.WriteString(w, `{"number":3,"body":"Risk: low — n\n\nDirect low-risk PR: yes\n","user":{"login":"a"},"base":{"sha":"`+baseSHA+`","repo":{"clone_url":"https://github.com/o/r.git"}},"head":{"sha":"`+headSHA+`"}}`)
		case "/repos/o/r/contents/.github/repo-policy.yml":
			writeContent(w, "contract: x\n")
		case "/repos/o/r/pulls/3/files":
			io.WriteString(w, `[]`)
		case "/repos/o/r/compare/" + baseSHA + "..." + headSHA:
			fmt.Fprintf(w, `{"merge_base_commit":{"sha":%q}}`, mergeSHA)
		case "/repos/o/r/issues/3/comments":
			io.WriteString(w, `[]`)
		case "/repos/o/r/pulls/3/reviews":
			io.WriteString(w, `[]`)
		case "/repos/o/r/commits/" + headSHA + "/check-runs":
			io.WriteString(w, `{"check_runs":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	event := map[string]any{
		"repository": map[string]any{"full_name": "o/r"},
		"issue": map[string]any{
			"pull_request": map[string]any{"url": server.URL + "/pr"},
		},
	}
	path := writeEvent(t, event)
	client := NewGitHubClient(testToken, server.URL, server.Client())
	state, err := BuildLiveState(context.Background(), path, testEnv(server.URL), client, stubGit{base: mergeSHA, digest: "sha256:" + strings.Repeat("0", 64)})
	if err != nil {
		t.Fatal(err)
	}
	if asMap(state["pull_request"])["number"] != 3 {
		t.Fatalf("%#v", state["pull_request"])
	}
	if state["issue"] != nil {
		t.Fatalf("direct PR should not set issue: %#v", state["issue"])
	}
}

func TestBuildLiveStateFailClosed(t *testing.T) {
	_, err := BuildLiveState(context.Background(), "nope.json", func(string) string { return "" }, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN is required") {
		t.Fatalf("token: %v", err)
	}
	env := func(key string) string {
		if key == "GITHUB_TOKEN" {
			return testToken
		}
		return ""
	}
	path := writeEvent(t, map[string]any{"foo": "bar"})
	_, err = BuildLiveState(context.Background(), path, env, NewGitHubClient(testToken, "http://127.0.0.1:1", http.DefaultClient), stubGit{})
	if err == nil || !strings.Contains(err.Error(), "GITHUB_REPOSITORY") && !strings.Contains(err.Error(), "not associated") {
		t.Fatalf("missing repo/pr: %v", err)
	}
}

func TestPermissionFailureDoesNotFailBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "repo-policy.yml"):
			writeContent(w, "contract: x\nchangelog:\n  root: changelog.d\n")
		case strings.Contains(r.URL.Path, "/files"):
			io.WriteString(w, `[]`)
		case strings.Contains(r.URL.Path, "/compare/"):
			fmt.Fprintf(w, `{"merge_base_commit":{"sha":%q}}`, mergeSHA)
		case strings.Contains(r.URL.Path, "/issues/4/comments"):
			io.WriteString(w, `[{"body":"repo-ops.merge-review.v1 verdict:merge-ready","user":{"login":"ghost"},"author_association":"MEMBER"}]`)
		case strings.Contains(r.URL.Path, "/reviews"):
			io.WriteString(w, `[]`)
		case strings.Contains(r.URL.Path, "/check-runs"):
			io.WriteString(w, `{"check_runs":[]}`)
		case strings.Contains(r.URL.Path, "/permission"):
			http.Error(w, "nope", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	event := map[string]any{
		"repository": map[string]any{"full_name": "o/r"},
		"pull_request": map[string]any{
			"number": 4,
			"body":   "Risk: low — n\n\nDirect low-risk PR: yes\n",
			"user":   map[string]any{"login": "a"},
			"base":   map[string]any{"sha": baseSHA, "repo": map[string]any{"clone_url": "https://github.com/o/r.git"}},
			"head":   map[string]any{"sha": headSHA},
		},
	}
	state, err := BuildLiveState(context.Background(), writeEvent(t, event), testEnv(server.URL), NewGitHubClient(testToken, server.URL, server.Client()), stubGit{base: mergeSHA, digest: "sha256:x"})
	if err != nil {
		t.Fatal(err)
	}
	comments, _ := state["comments"].([]any)
	if len(comments) != 1 || asMap(comments[0])["permission"] != nil {
		t.Fatalf("nil permission on failure: %#v", comments)
	}
}

func TestBuildLiveStateRetainsRenameAndHydratesFragment(t *testing.T) {
	renamed := "## Added\n- Renamed fragment.\n"
	copied := "## Changed\n- Copied fragment.\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "repo-policy.yml"):
			writeContent(w, "contract: x\nchangelog:\n  mode: fragments\n  root: changelog.d\n")
		case strings.Contains(r.URL.Path, "/files"):
			io.WriteString(w, `[
				{"filename":"changelog.d/3-new.md","previous_filename":"changelog.d/3-old.md","status":"renamed","additions":1,"deletions":0,"changes":1},
				{"filename":"changelog.d/3-copy.md","previous_filename":"changelog.d/3-src.md","status":"copied","additions":1,"deletions":0,"changes":1},
				{"filename":"README.md","previous_filename":"README.old","status":"renamed","additions":0,"deletions":0,"changes":0}
			]`)
		case strings.Contains(r.URL.Path, "/contents/changelog.d/3-new.md"):
			writeContent(w, renamed)
		case strings.Contains(r.URL.Path, "/contents/changelog.d/3-copy.md"):
			writeContent(w, copied)
		case strings.Contains(r.URL.Path, "/contents/changelog.d/"):
			http.Error(w, "should not load previous fragment", http.StatusNotFound)
		case strings.Contains(r.URL.Path, "/compare/"):
			fmt.Fprintf(w, `{"merge_base_commit":{"sha":%q}}`, mergeSHA)
		case strings.Contains(r.URL.Path, "/comments"):
			io.WriteString(w, `[]`)
		case strings.Contains(r.URL.Path, "/reviews"):
			io.WriteString(w, `[]`)
		case strings.Contains(r.URL.Path, "/check-runs"):
			io.WriteString(w, `{"check_runs":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	event := map[string]any{
		"repository": map[string]any{"full_name": "o/r"},
		"pull_request": map[string]any{
			"number": 4,
			"body":   "Risk: low — n\n\nDirect low-risk PR: yes\n",
			"user":   map[string]any{"login": "a"},
			"base":   map[string]any{"sha": baseSHA, "repo": map[string]any{"clone_url": "https://github.com/o/r.git"}},
			"head":   map[string]any{"sha": headSHA},
		},
	}
	state, err := BuildLiveState(context.Background(), writeEvent(t, event), testEnv(server.URL), NewGitHubClient(testToken, server.URL, server.Client()), stubGit{base: mergeSHA, digest: "sha256:x"})
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]map[string]any{}
	files, _ := state["files"].([]any)
	for _, item := range files {
		entry := asMap(item)
		if entry == nil {
			continue
		}
		byName[asString(entry["filename"])] = entry
	}
	if asString(byName["changelog.d/3-new.md"]["status"]) != "renamed" ||
		asString(byName["changelog.d/3-new.md"]["previous_filename"]) != "changelog.d/3-old.md" {
		t.Fatalf("renamed fragment %#v", byName["changelog.d/3-new.md"])
	}
	if asString(byName["changelog.d/3-copy.md"]["status"]) != "copied" ||
		asString(byName["changelog.d/3-copy.md"]["previous_filename"]) != "changelog.d/3-src.md" {
		t.Fatalf("copied fragment %#v", byName["changelog.d/3-copy.md"])
	}
	if asString(byName["README.md"]["previous_filename"]) != "README.old" {
		t.Fatalf("non-fragment rename %#v", byName["README.md"])
	}
	contents := asMap(state["file_contents"])
	if contents["changelog.d/3-new.md"] != renamed || contents["changelog.d/3-copy.md"] != copied {
		t.Fatalf("hydrated contents %#v", contents)
	}
	if _, ok := contents["changelog.d/3-old.md"]; ok {
		t.Fatal("previous rename path should not be hydrated")
	}
	if _, ok := contents["README.md"]; ok {
		t.Fatal("non-fragment should not be hydrated")
	}
}

type stubGit struct {
	base, digest string
	err          error
}

func (s stubGit) Metadata(context.Context, map[string]any, string, string) (string, string, error) {
	return s.base, s.digest, s.err
}

func testEnv(apiURL string) func(string) string {
	return func(key string) string {
		switch key {
		case "GITHUB_TOKEN":
			return testToken
		case "GITHUB_API_URL":
			return apiURL
		case "GITHUB_REPOSITORY":
			return "o/r"
		default:
			return ""
		}
	}
}

func writeEvent(t *testing.T, event map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "event.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeContent(w http.ResponseWriter, text string) {
	io.WriteString(w, `{"encoding":"base64","content":"`+base64.StdEncoding.EncodeToString([]byte(text))+`"}`)
}

func jsonString(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

func planApprovalBody(risk, intent, plan, commit string) string {
	return "**Plan approved**\n\n" +
		"- Risk: `" + risk + "`\n" +
		"- Intent digest: `" + intent + "`\n" +
		"- Plan digest: `" + plan + "`\n" +
		"- Plan commit: `" + commit + "`\n\n" +
		"Authoritative machine-readable record:\n\n" +
		"```text\n" +
		"repo-ops.plan-approval.v1 decision:approved risk:" + risk + " issue:42 intent:" + intent + " plan:" + plan + " plan-commit:" + commit + "\n" +
		"```\n"
}
