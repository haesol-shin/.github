package repositorypolicy_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

var workflowBundles = []string{
	"issue_comment-created-intent",
	"issue_comment-edited-intent",
	"issue_comment-deleted-intent",
}

var checkNames = []string{"contract", "merge approval"}

type workflowEvent struct {
	Action string `json:"action"`
	Issue  struct {
		Number      int            `json:"number"`
		PullRequest map[string]any `json:"pull_request"`
	} `json:"issue"`
	Comment struct {
		AuthorAssociation string `json:"author_association"`
		Body              string `json:"body"`
		User              struct {
			Login string `json:"login"`
		} `json:"user"`
	} `json:"comment"`
	Changes struct {
		Body struct {
			From string `json:"from"`
		} `json:"body"`
	} `json:"changes"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type workflowRecording struct {
	Method  string          `json:"method"`
	URL     string          `json:"url"`
	Headers map[string]any  `json:"headers"`
	Body    json.RawMessage `json:"body"`
	Request struct {
		Headers map[string]string `json:"headers"`
		Form    map[string]string `json:"form"`
	} `json:"request"`
}

type workflowCheckRun struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
}

type workflowExpected struct {
	Job                 string             `json:"job"`
	LinkedOpenPullHeads []string           `json:"linked_open_pull_heads"`
	CheckRuns           []workflowCheckRun `json:"check_runs"`
}

type recordedWorkflowAPI struct {
	t          *testing.T
	recordings []workflowRecording
	cursor     int
}

func TestWorkflowIntentRevocationReplay(t *testing.T) {
	root := filepath.Join("tests", "testdata", "events", "workflow")
	for _, name := range workflowBundles {
		t.Run(name, func(t *testing.T) {
			bundle := filepath.Join(root, name)
			var event workflowEvent
			var recordings []workflowRecording
			var want workflowExpected
			decodeJSONFile(t, filepath.Join(bundle, "webhook.json"), &event, false)
			decodeJSONFile(t, filepath.Join(bundle, "http", "recordings.json"), &recordings, false)
			decodeJSONFile(t, filepath.Join(bundle, "expected.json"), &want, false)
			if len(recordings) == 0 {
				t.Fatal("workflow replay has no HTTP recordings")
			}
			assertNoRecordedSecrets(t, bundle)
			got := replayIntentRevocation(t, event, recordings)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("workflow replay mismatch\n got: %#v\nwant: %#v", got, want)
			}
		})
	}
}

func TestPolicyWorkflowMatchesRevocationReplayContract(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(".github", "workflows", "policy.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(content)
	needles := []string{
		"  intent-revocation:",
		"github.event_name == 'issue_comment'",
		"!github.event.issue.pull_request",
		"github.event.comment.author_association == 'OWNER'",
		"github.event.comment.author_association == 'MEMBER'",
		"github.event.comment.author_association == 'COLLABORATOR'",
		"contains(github.event.comment.body, 'repo-ops.intent.v1')",
		"contains(github.event.comment.body, 'Intent accepted.')",
		"contains(github.event.changes.body.from, 'repo-ops.intent.v1')",
		"contains(github.event.changes.body.from, 'Intent accepted.')",
		"if [[ \"$AUTHOR_ASSOCIATION\" != \"OWNER\" ]]",
		"maintain|admin)",
		"/pulls?state=open&per_page=100",
		"for check_name in \"contract\" \"merge approval\"",
		"-f \"head_sha=${head_sha}\"",
		"-f \"conclusion=failure\"",
		"Intent authority changed",
	}
	for _, needle := range needles {
		if !strings.Contains(workflow, needle) {
			t.Errorf("policy workflow is missing %q", needle)
		}
	}
	if !regexp.MustCompile(`(?m)^permissions: \{\}\s*$`).MatchString(workflow) {
		t.Error("policy workflow must default to no permissions")
	}
	wantPermissions := map[string]string{
		"checks": "write", "contents": "read", "issues": "read", "pull-requests": "read",
	}
	for _, job := range []string{"publish", "intent-revocation"} {
		got := workflowJobPermissions(t, workflowJobBlock(t, workflow, job))
		if !reflect.DeepEqual(got, wantPermissions) {
			t.Errorf("%s permissions mismatch: got %#v, want %#v", job, got, wantPermissions)
		}
	}
}

func TestIntentRevocationRejectsUnauthorizedEvents(t *testing.T) {
	var event workflowEvent
	decodeJSONFile(t, filepath.Join("tests", "testdata", "events", "workflow", workflowBundles[0], "webhook.json"), &event, false)
	if !intentRevocationSelected(event) {
		t.Fatal("trusted issue comment was not selected")
	}
	event.Issue.PullRequest = map[string]any{"url": "https://example.invalid/pull/1"}
	if intentRevocationSelected(event) {
		t.Fatal("pull request comment selected intent revocation")
	}
	event.Issue.PullRequest = nil
	event.Comment.AuthorAssociation = "NONE"
	if intentRevocationSelected(event) {
		t.Fatal("untrusted author selected intent revocation")
	}
}

func replayIntentRevocation(t *testing.T, event workflowEvent, recordings []workflowRecording) workflowExpected {
	t.Helper()
	if !intentRevocationSelected(event) {
		t.Fatal("workflow event would not select intent revocation")
	}
	if event.Repository.FullName == "" || !strings.Contains(event.Repository.FullName, "/") {
		t.Fatal("webhook repository.full_name is invalid")
	}
	api := &recordedWorkflowAPI{t: t, recordings: recordings}
	if !authorizeWorkflowEvent(api, event) {
		t.Fatal("recorded trusted event was not authorized")
	}
	heads := linkedWorkflowHeads(t, api.listOpenPulls(event.Repository.FullName), event.Issue.Number)
	observed := workflowExpected{Job: "intent-revocation", LinkedOpenPullHeads: heads, CheckRuns: []workflowCheckRun{}}
	for _, head := range heads {
		for _, name := range checkNames {
			form := map[string]string{
				"name": name, "head_sha": head, "status": "completed", "conclusion": "failure",
				"output[title]":   "Intent authority changed",
				"output[summary]": "Policy checks revoked until the linked pull request is revalidated.",
			}
			recording := api.take("POST", "/repos/"+event.Repository.FullName+"/check-runs", form)
			var body workflowCheckRun
			if err := json.Unmarshal(recording.Body, &body); err != nil {
				t.Fatal(err)
			}
			want := workflowCheckRun{Name: name, Conclusion: "failure", HeadSHA: head}
			if body != want {
				t.Fatalf("recorded check run mismatch: got %#v, want %#v", body, want)
			}
			observed.CheckRuns = append(observed.CheckRuns, want)
		}
	}
	if api.cursor != len(api.recordings) {
		t.Fatalf("workflow replay left %d unused recordings", len(api.recordings)-api.cursor)
	}
	return observed
}

func intentRevocationSelected(event workflowEvent) bool {
	if event.Issue.PullRequest != nil {
		return false
	}
	switch event.Comment.AuthorAssociation {
	case "OWNER", "MEMBER", "COLLABORATOR":
	default:
		return false
	}
	return containsIntentMarker(event.Comment.Body) || containsIntentMarker(event.Changes.Body.From)
}

func containsIntentMarker(value string) bool {
	return strings.Contains(value, "repo-ops.intent.v1") || strings.Contains(value, "Intent accepted.")
}

func authorizeWorkflowEvent(api *recordedWorkflowAPI, event workflowEvent) bool {
	if event.Comment.AuthorAssociation == "OWNER" {
		return true
	}
	recording := api.take("GET", "/repos/"+event.Repository.FullName+"/collaborators/"+event.Comment.User.Login+"/permission", nil)
	var body struct {
		Permission string `json:"permission"`
	}
	if err := json.Unmarshal(recording.Body, &body); err != nil {
		api.t.Fatal(err)
	}
	return body.Permission == "maintain" || body.Permission == "admin"
}

func (api *recordedWorkflowAPI) listOpenPulls(repository string) []map[string]any {
	path := "/repos/" + repository + "/pulls?state=open&per_page=100"
	var pulls []map[string]any
	for path != "" {
		recording := api.take("GET", path, nil)
		var page []map[string]any
		if err := json.Unmarshal(recording.Body, &page); err != nil {
			api.t.Fatal(err)
		}
		pulls = append(pulls, page...)
		path = nextRecordedURL(recording.Headers)
	}
	return pulls
}

func (api *recordedWorkflowAPI) take(method, path string, form map[string]string) workflowRecording {
	api.t.Helper()
	if api.cursor >= len(api.recordings) {
		api.t.Fatalf("no recording left for %s %s", method, path)
	}
	recording := api.recordings[api.cursor]
	api.cursor++
	if recording.Method != method || normalizeRecordedURL(recording.URL) != path {
		api.t.Fatalf("recording mismatch: got %s %s, want %s %s", recording.Method, recording.URL, method, path)
	}
	if form != nil && !reflect.DeepEqual(recording.Request.Form, form) {
		api.t.Fatalf("recorded form mismatch\n got: %#v\nwant: %#v", recording.Request.Form, form)
	}
	return recording
}

func linkedWorkflowHeads(t *testing.T, pulls []map[string]any, issue int) []string {
	t.Helper()
	link := regexp.MustCompile(`(?im)^(Fixes|Closes|Related)[[:space:]]+#` + fmt.Sprint(issue) + `[[:space:]]*$`)
	hex40 := regexp.MustCompile(`^[0-9a-f]{40}$`)
	var heads []string
	for _, pull := range pulls {
		body, _ := pull["body"].(string)
		if !link.MatchString(body) {
			continue
		}
		head, _ := pull["head"].(map[string]any)
		sha, _ := head["sha"].(string)
		if !hex40.MatchString(sha) {
			t.Fatalf("linked pull head is not 40-hex: %q", sha)
		}
		heads = append(heads, sha)
	}
	return heads
}

func nextRecordedURL(headers map[string]any) string {
	for key, value := range headers {
		if !strings.EqualFold(key, "link") {
			continue
		}
		for _, part := range strings.Split(fmt.Sprint(value), ",") {
			if !strings.Contains(part, `rel="next"`) && !strings.Contains(part, "rel=next") {
				continue
			}
			start, end := strings.Index(part, "<"), strings.Index(part, ">")
			if start >= 0 && end > start {
				return normalizeRecordedURL(part[start+1 : end])
			}
		}
	}
	return ""
}

func normalizeRecordedURL(raw string) string {
	raw = strings.ReplaceAll(raw, "{api}", "")
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Host != "" {
		return parsed.RequestURI()
	}
	if strings.HasPrefix(raw, "/") {
		return raw
	}
	return "/" + raw
}

func assertNoRecordedSecrets(t *testing.T, bundle string) {
	t.Helper()
	liveSecret := regexp.MustCompile(`(?i)(ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|ghs_[A-Za-z0-9]{20,}|bearer\s+(ghp_|github_pat_|ghs_)\S+|x-access-token:(ghp_|github_pat_|ghs_)\S+)`)
	for _, relative := range []string{"webhook.json", filepath.Join("http", "recordings.json"), "expected.json"} {
		path := filepath.Join(bundle, relative)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if relative == filepath.Join("http", "recordings.json") && !strings.Contains(string(content), "REDACTED") {
			t.Fatalf("%s must contain redacted authorization", path)
		}
		if liveSecret.Match(content) {
			t.Fatalf("%s contains a live-looking secret", path)
		}
	}
}

func workflowJobBlock(t *testing.T, workflow, name string) string {
	t.Helper()
	start := strings.Index(workflow, "  "+name+":\n")
	if start < 0 {
		t.Fatalf("policy workflow is missing job %s", name)
	}
	body := workflow[start+len("  "+name+":\n"):]
	next := regexp.MustCompile(`(?m)^  [A-Za-z0-9_-]+:\n`).FindStringIndex(body)
	if next != nil {
		body = body[:next[0]]
	}
	return body
}

func workflowJobPermissions(t *testing.T, block string) map[string]string {
	t.Helper()
	lines := strings.Split(block, "\n")
	for index, line := range lines {
		if line != "    permissions:" {
			continue
		}
		permissions := map[string]string{}
		for _, entry := range lines[index+1:] {
			if !strings.HasPrefix(entry, "      ") {
				break
			}
			key, value, found := strings.Cut(strings.TrimSpace(entry), ":")
			if !found {
				t.Fatalf("invalid permission entry %q", entry)
			}
			permissions[key] = strings.TrimSpace(value)
		}
		return permissions
	}
	t.Fatal("workflow job is missing permissions")
	return nil
}
