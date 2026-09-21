package collect

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/haesol-shin/.github/internal/canonical"
)

func BuildLiveState(ctx context.Context, eventPath string, getenv func(string) string, gh *GitHubClient, git GitMetadataRunner) (map[string]any, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	token := getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is required for live validation")
	}
	if gh == nil {
		gh = NewGitHubClient(token, getenv("GITHUB_API_URL"), nil)
	}
	if git == nil {
		git = NewGitRunner()
	}

	raw, err := os.ReadFile(eventPath)
	if err != nil {
		return nil, err
	}
	decoded, err := decodeJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	event, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("event is not associated with a pull request")
	}

	repository := ""
	if repo := asMap(event["repository"]); repo != nil {
		repository, _ = repo["full_name"].(string)
	}
	if repository == "" {
		repository = getenv("GITHUB_REPOSITORY")
	}
	if repository == "" {
		return nil, fmt.Errorf("GITHUB_REPOSITORY is required for live validation")
	}

	pullRequest := asMap(event["pull_request"])
	if pullRequest == nil {
		if issue := asMap(event["issue"]); issue != nil {
			if pr := asMap(issue["pull_request"]); pr != nil {
				if prURL, ok := pr["url"].(string); ok && prURL != "" {
					fetched, err := gh.Get(ctx, prURL, "")
					if err != nil {
						return nil, err
					}
					pullRequest = asMap(fetched)
				}
			}
		}
	}
	if pullRequest == nil {
		return nil, fmt.Errorf("event is not associated with a pull request")
	}

	number, ok := lookupInt(pullRequest["number"])
	if !ok {
		return nil, fmt.Errorf("pull request number is required")
	}
	baseSHA, ok := nestedString(pullRequest, "base", "sha")
	if !ok || baseSHA == "" {
		return nil, fmt.Errorf("pull request base SHA is required")
	}
	headSHA, ok := nestedString(pullRequest, "head", "sha")
	if !ok || headSHA == "" {
		return nil, fmt.Errorf("pull request head SHA is required")
	}
	body := orEmptyString(pullRequest["body"])

	policyText, err := APIContent(ctx, gh, repository, ".github/repo-policy.yml", baseSHA)
	if err != nil {
		return nil, err
	}
	policy, err := DecodePolicy(policyText)
	if err != nil {
		return nil, err
	}

	fileItems, err := gh.GetAll(ctx, fmt.Sprintf("/repos/%s/pulls/%d/files?per_page=100", repository, number), "", "")
	if err != nil {
		return nil, err
	}
	files := make([]any, 0, len(fileItems))
	for _, item := range fileItems {
		obj := asMap(item)
		if obj == nil {
			continue
		}
		filename, ok := obj["filename"].(string)
		if !ok {
			continue
		}
		entry := map[string]any{
			"filename":  filename,
			"status":    obj["status"],
			"additions": obj["additions"],
			"deletions": obj["deletions"],
			"changes":   obj["changes"],
		}
		if previous, ok := obj["previous_filename"].(string); ok && previous != "" {
			entry["previous_filename"] = previous
		}
		files = append(files, entry)
	}

	fragmentRoot := "changelog.d"
	if policyMap := asMap(policy); policyMap != nil {
		if changelog := asMap(policyMap["changelog"]); changelog != nil {
			if root, exists := changelog["root"]; exists {
				rootStr, ok := root.(string)
				if !ok {
					return nil, fmt.Errorf("repository policy changelog.root must be a string")
				}
				fragmentRoot = rootStr
			}
		}
	}

	fragmentContents := map[string]any{}
	for _, entryAny := range files {
		entry := asMap(entryAny)
		status := ""
		if s, ok := entry["status"].(string); ok {
			status = strings.ToLower(s)
		}
		filename, _ := entry["filename"].(string)
		if status != "" && status != "removed" && fragmentPath(filename, fragmentRoot) {
			content, err := APIContent(ctx, gh, repository, filename, headSHA)
			if err != nil {
				return nil, err
			}
			fragmentContents[filename] = content
		}
	}

	comparison, err := gh.Get(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", repository, baseSHA, headSHA), "")
	if err != nil {
		return nil, err
	}
	mergeBaseCommit := asMap(asMap(comparison)["merge_base_commit"])
	if mergeBaseCommit == nil {
		return nil, fmt.Errorf("compare response is missing merge_base_commit")
	}
	mergeBaseSHA, _ := mergeBaseCommit["sha"].(string)
	if mergeBaseSHA == "" {
		return nil, fmt.Errorf("compare response is missing merge_base_commit")
	}
	mergeBase, diffDigest, err := git.Metadata(ctx, pullRequest, token, mergeBaseSHA)
	if err != nil {
		return nil, err
	}

	permissionCache := map[string]*string{}
	hydrated := func(item map[string]any, permissionRequired bool) map[string]any {
		itemBody := orEmptyString(item["body"])
		var login string
		if user := asMap(item["user"]); user != nil {
			login, _ = user["login"].(string)
		}
		var permission *string
		if permissionRequired || strings.Contains(itemBody, "repo-ops.intent.v1") ||
			strings.Contains(itemBody, "repo-ops.plan-approval.v1") ||
			strings.Contains(itemBody, "repo-ops.merge-review.v1") {
			permission = collaboratorPermission(ctx, gh, repository, login, permissionCache)
		}
		return entryFromAPI(item, permission)
	}

	commentItems, err := gh.GetAll(ctx, fmt.Sprintf("/repos/%s/issues/%d/comments?per_page=100", repository, number), "", "")
	if err != nil {
		return nil, err
	}
	prComments := make([]any, 0, len(commentItems))
	for _, item := range commentItems {
		obj := asMap(item)
		if obj == nil {
			return nil, fmt.Errorf("paginated GitHub API response is not a list: comments")
		}
		prComments = append(prComments, hydrated(obj, false))
	}

	reviewItems, err := gh.GetAll(ctx, fmt.Sprintf("/repos/%s/pulls/%d/reviews?per_page=100", repository, number), "", "")
	if err != nil {
		return nil, err
	}
	reviews := make([]any, 0, len(reviewItems))
	for _, item := range reviewItems {
		obj := asMap(item)
		if obj == nil {
			continue
		}
		if orEmptyString(obj["body"]) == "" {
			continue
		}
		if asString(obj["state"]) == "DISMISSED" {
			continue
		}
		reviews = append(reviews, hydrated(obj, false))
	}

	var issue any
	var issueBody any
	intentComments := []any{}
	if issueNumber := parseRelatedIssue(body); issueNumber != nil {
		issue = issueNumber
		issueDocument, err := gh.Get(ctx, fmt.Sprintf("/repos/%s/issues/%v", repository, issueNumber), "")
		if err != nil {
			return nil, err
		}
		issueObj := asMap(issueDocument)
		issueBody = orEmptyString(issueObj["body"])
		intentItems, err := gh.GetAll(ctx, fmt.Sprintf("/repos/%s/issues/%v/comments?per_page=100", repository, issueNumber), "", "")
		if err != nil {
			return nil, err
		}
		intentComments = make([]any, 0, len(intentItems))
		for _, item := range intentItems {
			obj := asMap(item)
			if obj == nil {
				return nil, fmt.Errorf("paginated GitHub API response is not a list: intent comments")
			}
			intentComments = append(intentComments, hydrated(obj, true))
		}
	}

	planPath := parsePlanPath(body)
	currentPlanDigest := "none"
	if planPath != "" {
		planText, err := APIContent(ctx, gh, repository, planPath, headSHA)
		if err != nil {
			return nil, err
		}
		digest, err := canonical.PlanDigest(planText)
		if err != nil {
			return nil, err
		}
		currentPlanDigest = digest
	}

	approvalCommits := []any{}
	for _, commentAny := range append(append([]any{}, prComments...), reviews...) {
		comment := asMap(commentAny)
		if comment == nil || !recordCandidate(comment) {
			continue
		}
		approval := parsePlanApproval(orEmptyString(comment["body"]))
		if approval == nil {
			continue
		}
		planCommit := asString(approval["plan-commit"])
		approvalCompare, err := gh.Get(ctx, fmt.Sprintf("/repos/%s/compare/%s...%s", repository, planCommit, headSHA), "")
		if err != nil {
			return nil, err
		}
		var approvedPlanDigest any
		if planPath != "" {
			planText, err := APIContent(ctx, gh, repository, planPath, planCommit)
			if err != nil {
				return nil, err
			}
			digest, err := canonical.PlanDigest(planText)
			if err != nil {
				return nil, err
			}
			approvedPlanDigest = digest
		}
		status := asString(asMap(approvalCompare)["status"])
		if (status == "ahead" || status == "identical") &&
			approvedPlanDigest == approval["plan"] &&
			approvedPlanDigest == currentPlanDigest {
			approvalCommits = append(approvalCommits, planCommit)
		}
	}

	checkRuns, err := gh.GetAll(ctx, fmt.Sprintf("/repos/%s/commits/%s/check-runs?per_page=100", repository, headSHA), "check_runs", "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var qualityName any
	if policyMap := asMap(policy); policyMap != nil {
		if quality := asMap(policyMap["quality"]); quality != nil {
			qualityName = quality["check"]
		}
	}
	var matching []map[string]any
	for _, run := range checkRuns {
		obj := asMap(run)
		if obj == nil {
			continue
		}
		if obj["name"] == qualityName {
			matching = append(matching, obj)
		}
	}
	var conclusion any
	if len(matching) > 0 {
		conclusion = matching[len(matching)-1]["conclusion"]
	}
	quality := map[string]any{
		"name":       qualityName,
		"conclusion": conclusion,
	}

	var author any
	if user := asMap(pullRequest["user"]); user != nil {
		author = user["login"]
	}

	return map[string]any{
		"policy": policy,
		"pull_request": map[string]any{
			"number":      number,
			"author":      author,
			"body":        body,
			"base":        baseSHA,
			"merge_base":  mergeBase,
			"head":        headSHA,
			"diff_digest": diffDigest,
			"files":       files,
		},
		"files":           files,
		"file_contents":   fragmentContents,
		"issue":           issue,
		"issue_body":      issueBody,
		"intent_comments": intentComments,
		"comments":        prComments,
		"reviews":         reviews,
		"plan_digest":     currentPlanDigest,
		"plan_commits":    approvalCommits,
		"quality":         quality,
	}, nil
}
