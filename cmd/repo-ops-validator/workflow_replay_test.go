package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

type workflowCheckRun struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
}

func TestIntentRevocationWorkflowReplay(t *testing.T) {
	root := filepath.Join(testRepoRoot(), "tests", "testdata", "events", "workflow")
	for _, name := range []string{
		"issue_comment-created-intent",
		"issue_comment-edited-intent",
		"issue_comment-deleted-intent",
	} {
		t.Run(name, func(t *testing.T) {
			replayIntentRevocation(t, filepath.Join(root, name))
		})
	}
}

func replayIntentRevocation(t *testing.T, bundle string) {
	t.Helper()
	var event struct {
		Issue struct {
			Number      int `json:"number"`
			PullRequest any `json:"pull_request"`
		} `json:"issue"`
		Comment struct {
			Body              string `json:"body"`
			AuthorAssociation string `json:"author_association"`
		} `json:"comment"`
		Changes struct {
			Body struct {
				From string `json:"from"`
			} `json:"body"`
		} `json:"changes"`
	}
	readReplayJSON(t, filepath.Join(bundle, "webhook.json"), &event)
	if event.Issue.PullRequest != nil {
		t.Fatal("workflow-only event unexpectedly targets a pull request")
	}
	if event.Comment.AuthorAssociation != "OWNER" &&
		event.Comment.AuthorAssociation != "MEMBER" &&
		event.Comment.AuthorAssociation != "COLLABORATOR" {
		t.Fatalf("unauthorized association %q", event.Comment.AuthorAssociation)
	}
	if !containsIntentMarker(event.Comment.Body) && !containsIntentMarker(event.Changes.Body.From) {
		t.Fatal("event would not select intent-revocation")
	}

	type recording struct {
		Method  string          `json:"method"`
		URL     string          `json:"url"`
		Body    json.RawMessage `json:"body"`
		Request struct {
			Form map[string]string `json:"form"`
		} `json:"request"`
	}
	var recordings []recording
	readReplayJSON(t, filepath.Join(bundle, "http", "recordings.json"), &recordings)
	var heads []string
	var posts []workflowCheckRun
	authorized := event.Comment.AuthorAssociation == "OWNER"
	sawPermission := false
	link := regexp.MustCompile(fmt.Sprintf(`(?m)^(?:Fixes|Closes|Related)\s+#%d\s*$`, event.Issue.Number))
	headPattern := regexp.MustCompile(`^[0-9a-f]{40}$`)
	for _, item := range recordings {
		switch {
		case item.Method == "GET" && strings.Contains(item.URL, "/collaborators/"):
			if event.Comment.AuthorAssociation == "OWNER" {
				t.Fatal("owner authorization unexpectedly queried collaborator permission")
			}
			var permission struct {
				Value string `json:"permission"`
			}
			if err := json.Unmarshal(item.Body, &permission); err != nil {
				t.Fatalf("decode collaborator permission: %v", err)
			}
			sawPermission = true
			authorized = permission.Value == "maintain" || permission.Value == "admin"
		case item.Method == "GET" && strings.Contains(item.URL, "/pulls?"):
			if !authorized {
				t.Fatal("workflow continued after collaborator authorization failed")
			}
			var pulls []struct {
				Body string `json:"body"`
				Head struct {
					SHA string `json:"sha"`
				} `json:"head"`
			}
			if err := json.Unmarshal(item.Body, &pulls); err != nil {
				t.Fatalf("decode pull recording: %v", err)
			}
			for _, pull := range pulls {
				if link.MatchString(pull.Body) {
					if !headPattern.MatchString(pull.Head.SHA) {
						t.Fatalf("linked pull head is not 40-hex: %q", pull.Head.SHA)
					}
					heads = append(heads, pull.Head.SHA)
				}
			}
		case item.Method == "POST" && strings.HasSuffix(item.URL, "/check-runs"):
			if !authorized {
				t.Fatal("workflow posted a revocation after authorization failed")
			}
			form := item.Request.Form
			if form["status"] != "completed" || form["conclusion"] != "failure" ||
				form["output[title]"] != "Intent authority changed" ||
				form["output[summary]"] != "Policy checks revoked until the linked pull request is revalidated." {
				t.Fatalf("invalid revocation form: %v", form)
			}
			posts = append(posts, workflowCheckRun{
				Name:       form["name"],
				Conclusion: form["conclusion"],
				HeadSHA:    form["head_sha"],
			})
		default:
			t.Fatalf("unused workflow recording: %s %s", item.Method, item.URL)
		}
	}
	if event.Comment.AuthorAssociation != "OWNER" && !sawPermission {
		t.Fatal("non-owner authorization did not query collaborator permission")
	}

	var expected struct {
		Job                 string             `json:"job"`
		LinkedOpenPullHeads []string           `json:"linked_open_pull_heads"`
		CheckRuns           []workflowCheckRun `json:"check_runs"`
	}
	readReplayJSON(t, filepath.Join(bundle, "expected.json"), &expected)
	if expected.Job != "intent-revocation" {
		t.Fatalf("job = %q", expected.Job)
	}
	if !slices.Equal(heads, expected.LinkedOpenPullHeads) {
		t.Fatalf("linked heads = %v, want %v", heads, expected.LinkedOpenPullHeads)
	}
	if !slices.Equal(posts, expected.CheckRuns) {
		t.Fatalf("check runs = %v, want %v", posts, expected.CheckRuns)
	}
	if len(posts) != len(heads)*2 {
		t.Fatalf("got %d check runs for %d linked heads", len(posts), len(heads))
	}
	for index, head := range heads {
		pair := posts[index*2 : index*2+2]
		if pair[0].Name != "contract" || pair[1].Name != "merge approval" ||
			pair[0].HeadSHA != head || pair[1].HeadSHA != head {
			t.Fatalf("check pair for %s = %v", head, pair)
		}
	}
}

func containsIntentMarker(text string) bool {
	return strings.Contains(text, "repo-ops.intent.v1") || strings.Contains(text, "Intent accepted.")
}
