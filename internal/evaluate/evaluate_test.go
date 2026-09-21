package evaluate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contractRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "contracts", "v0.1.0")
}

func loadJSON(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func cloneState(t *testing.T, state map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func findings(t *testing.T, state map[string]any, check Check) []string {
	t.Helper()
	return ValidateState(state, contractRoot(t), check)
}

func containsFinding(findings []string, substr string) bool {
	for _, finding := range findings {
		if strings.Contains(finding, substr) {
			return true
		}
	}
	return false
}

func replaceBody(state map[string]any, old, neu string) {
	pr := asMap(state["pull_request"])
	pr["body"] = strings.Replace(asString(pr["body"]), old, neu, 1)
}

func commentAt(state map[string]any, index int) map[string]any {
	return asEntries(state["comments"])[index]
}

func intentAt(state map[string]any, index int) map[string]any {
	return asEntries(state["intent_comments"])[index]
}

func v1State(t *testing.T) map[string]any {
	t.Helper()
	state := cloneState(t, loadJSON(t, "valid-high.json"))
	replaceBody(state, "Fixes #42", "Related #42")
	legacy := asString(intentAt(state, 0)["body"])
	v1 := "**Intent accepted**\n\n" +
		"Add a trusted validator without executing pull request code.\n\n" +
		"- Risk: `high`\n" +
		"- Intent digest: `sha256:0dbcd01f3a688d6e6c688048de2f13c525e3f6df968573ae7f5c4fbbdeae59dc`\n" +
		"- Supersedes: `sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871`\n\n" +
		"Authoritative machine-readable record:\n\n" +
		"```text\n" +
		"repo-ops.intent.v1 decision:accepted risk:high intent:sha256:0dbcd01f3a688d6e6c688048de2f13c525e3f6df968573ae7f5c4fbbdeae59dc supersedes:sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871\n" +
		"```\n"
	state["intent_comments"] = []any{
		map[string]any{"author": "maintainer", "association": "OWNER", "permission": "admin", "body": legacy},
		map[string]any{"author": "maintainer", "association": "OWNER", "permission": "admin", "body": v1},
	}
	commentAt(state, 0)["body"] = strings.ReplaceAll(
		asString(commentAt(state, 0)["body"]),
		"sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871",
		"sha256:0dbcd01f3a688d6e6c688048de2f13c525e3f6df968573ae7f5c4fbbdeae59dc",
	)
	commentAt(state, 1)["body"] = strings.ReplaceAll(
		asString(commentAt(state, 1)["body"]),
		"sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871",
		"sha256:0dbcd01f3a688d6e6c688048de2f13c525e3f6df968573ae7f5c4fbbdeae59dc",
	)
	return state
}

const (
	fixtureChangelog     = "repo-ops.changelog.v1 kind:not-required value:receipt-validation"
	exactlyOneChangelog  = "pull request body must contain exactly one repo-ops.changelog.v1 record"
	malformedChangelog   = "malformed repo-ops.changelog.v1 record"
	changelogSchemaLabel = "changelog declaration"
)

var changelogLifecycleFindings = []string{
	"required changelog declarations need an added or modified fragment",
	"ordinary pull requests must not edit CHANGELOG.md",
	"fragment deletion is reserved for release changelog declarations",
	"release changelog declarations must update CHANGELOG.md",
	"release changelog declarations must consume at least one fragment",
	"release changelog declarations must consume, not modify, fragments",
}

func withChangelogLine(t *testing.T, line string, files []any, contents map[string]any, suffix string) map[string]any {
	t.Helper()
	state := v1State(t)
	replaceBody(state, fixtureChangelog, line)
	if suffix != "" {
		pr := asMap(state["pull_request"])
		pr["body"] = asString(pr["body"]) + suffix
	}
	if files != nil {
		state["files"] = files
	}
	if contents != nil {
		state["file_contents"] = contents
	}
	return state
}

func ownedFragment() ([]any, map[string]any) {
	return []any{
		map[string]any{"filename": "changelog.d/42-validator.md", "status": "added"},
	}, map[string]any{
		"changelog.d/42-validator.md": "## Changed\n- Validate the trusted policy contract.\n",
	}
}

func changelogBait() []any {
	return []any{map[string]any{"filename": "CHANGELOG.md", "status": "modified"}}
}

func assertNoChangelogLifecycle(t *testing.T, got []string) {
	t.Helper()
	joined := strings.Join(got, "\n")
	for _, needle := range changelogLifecycleFindings {
		if strings.Contains(joined, needle) {
			t.Fatalf("lifecycle finding %q leaked into %v", needle, got)
		}
	}
}

func TestValidHighRiskRoute(t *testing.T) {
	got := findings(t, loadJSON(t, "valid-high.json"), CheckAll)
	if len(got) != 0 {
		t.Fatalf("valid high-risk fixture: %v", got)
	}
}

func TestValidLowDirectRoute(t *testing.T) {
	got := findings(t, loadJSON(t, "valid-low-direct.json"), CheckAll)
	if len(got) != 0 {
		t.Fatalf("valid low-direct fixture: %v", got)
	}
}

func TestLowRiskPlanIsOptional(t *testing.T) {
	state := cloneState(t, loadJSON(t, "valid-low-direct.json"))
	digest := "sha256:" + strings.Repeat("d", 64)
	state["plan_digest"] = digest
	commentAt(state, 0)["body"] = strings.Replace(asString(commentAt(state, 0)["body"]), "plan:none", "plan:"+digest, 1)
	got := findings(t, state, CheckAll)
	if len(got) != 0 {
		t.Fatalf("optional low-risk plan: %v", got)
	}
}

func TestHighRiskPlanApprovalRequiresOwner(t *testing.T) {
	state := v1State(t)
	commentAt(state, 0)["association"] = "MEMBER"
	got := findings(t, state, CheckContract)
	if !containsFinding(got, "high-risk plan approval was not posted by an authorized maintainer") {
		t.Fatalf("expected owner-authority finding, got %v", got)
	}
}

func TestHighRiskAdminPermissionIsNotPlanAuthority(t *testing.T) {
	state := v1State(t)
	commentAt(state, 0)["association"] = "MEMBER"
	commentAt(state, 0)["permission"] = "admin"
	got := findings(t, state, CheckContract)
	if !containsFinding(got, "high-risk plan approval was not posted by an authorized maintainer") {
		t.Fatalf("admin is not high-risk plan authority, got %v", got)
	}
}

func TestAdvisoryHeadingsAreNotFindings(t *testing.T) {
	state := cloneState(t, loadJSON(t, "valid-low-direct.json"))
	replaceBody(state, "## Summary", "Removed ## Summary")
	replaceBody(state, "## Changes", "Removed ## Changes")
	got := findings(t, state, CheckAll)
	if len(got) != 0 {
		t.Fatalf("generic headings must stay advisory, got %v", got)
	}

	issueState := cloneState(t, loadJSON(t, "valid-high.json"))
	issueState["issue_body"] = strings.Replace(asString(issueState["issue_body"]), "## Problem", "Removed ## Problem", 1)
	got = findings(t, issueState, CheckAll)
	if len(got) != 0 {
		t.Fatalf("issue headings must stay advisory, got %v", got)
	}
}

func TestIntentSupersession(t *testing.T) {
	state := v1State(t)
	body := asString(intentAt(state, 1)["body"])
	intentAt(state, 1)["body"] = strings.ReplaceAll(strings.ReplaceAll(body,
		"supersedes:sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871",
		"supersedes:none"),
		"- Supersedes: `sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871`",
		"- Supersedes: `none`")
	got := findings(t, state, CheckContract)
	if !containsFinding(got, "repo-ops.intent.v1 supersedes does not match the current intent") {
		t.Fatalf("expected supersession finding, got %v", got)
	}
}

func TestFirstV1WithoutLegacyUsesNone(t *testing.T) {
	state := v1State(t)
	intents := asEntries(state["intent_comments"])
	body := asString(intents[1]["body"])
	intents[1]["body"] = strings.ReplaceAll(strings.ReplaceAll(body,
		"supersedes:sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871",
		"supersedes:none"),
		"- Supersedes: `sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871`",
		"- Supersedes: `none`")
	state["intent_comments"] = []any{intents[1]}
	got := findings(t, state, CheckAll)
	if len(got) != 0 {
		t.Fatalf("first v1 with none: %v", got)
	}
}

func TestContractCheckExcludesMergeReceipt(t *testing.T) {
	state := v1State(t)
	state["comments"] = []any{commentAt(state, 0)}
	if got := findings(t, state, CheckContract); len(got) != 0 {
		t.Fatalf("contract check should ignore missing receipt, got %v", got)
	}
	got := findings(t, state, CheckAll)
	if !containsFinding(got, "no merge-ready receipt was found") {
		t.Fatalf("all check should require receipt, got %v", got)
	}
	merge := findings(t, state, CheckMergeApproval)
	if !containsFinding(merge, "no merge-ready receipt was found") {
		t.Fatalf("merge-approval includes receipt, got %v", merge)
	}
}

func TestChangelogOwnership(t *testing.T) {
	files, contents := ownedFragment()
	state := withChangelogLine(t, "repo-ops.changelog.v1 kind:required value:fragment-added", files, contents, "")
	if got := findings(t, state, CheckContract); len(got) != 0 {
		t.Fatalf("owned fragment should pass, got %v", got)
	}

	owned := cloneState(t, state)
	asEntries(owned["files"])[0]["filename"] = "changelog.d/41-validator.md"
	owned["file_contents"] = map[string]any{
		"changelog.d/41-validator.md": "## Changed\n- Validate the trusted policy contract.\n",
	}
	got := findings(t, owned, CheckContract)
	if !containsFinding(got, "issue-backed fragment filename must begin with the linked issue number") {
		t.Fatalf("expected ownership finding, got %v", got)
	}
}

func TestFragmentSectionFinding(t *testing.T) {
	files := []any{map[string]any{"filename": "changelog.d/42-validator.md", "status": "added"}}
	contents := map[string]any{"changelog.d/42-validator.md": "## Notes\n- Unsupported section.\n"}
	state := withChangelogLine(t, "repo-ops.changelog.v1 kind:required value:fragment-added", files, contents, "")
	got := findings(t, state, CheckContract)
	if !containsFinding(got, "content must follow an allowed section heading") {
		t.Fatalf("expected fragment section finding, got %v", got)
	}
}

func TestReleaseAcceptsTrustedBaseFragmentDeletion(t *testing.T) {
	for _, filename := range []string{
		"changelog.d/42-validator.md",
		"changelog.d/3-repo-ops-v0.1.0.md",
	} {
		t.Run(filename, func(t *testing.T) {
			files := []any{
				map[string]any{"filename": "CHANGELOG.md", "status": "added"},
				map[string]any{"filename": filename, "status": "removed"},
			}
			state := withChangelogLine(t, "repo-ops.changelog.v1 kind:release value:v0.1.0", files, nil, "")
			got := findings(t, state, CheckContract)
			if len(got) != 0 {
				t.Fatalf("release deletion of %s: %v", filename, got)
			}
		})
	}
}

func TestRejectsInvalidIncomingFragmentFilename(t *testing.T) {
	for _, status := range []string{"added", "modified", "renamed", "copied"} {
		t.Run(status, func(t *testing.T) {
			files := []any{
				map[string]any{"filename": "changelog.d/3-repo-ops-v0.1.0.md", "status": status},
			}
			contents := map[string]any{
				"changelog.d/3-repo-ops-v0.1.0.md": "## Fixed\n- Consume a legacy trusted-base fragment.\n",
			}
			state := withChangelogLine(t, "repo-ops.changelog.v1 kind:required value:fragment-added", files, contents, "")
			got := findings(t, state, CheckContract)
			if !containsFinding(got, "fragment filename must use <issue>-<slug>.md or direct-<slug>.md") {
				t.Fatalf("expected incoming filename rejection, got %v", got)
			}
		})
	}
}

func TestReleaseRenameCannotBypassIncomingFilenameValidation(t *testing.T) {
	for _, status := range []string{"renamed", "copied"} {
		t.Run(status, func(t *testing.T) {
			files := []any{
				map[string]any{"filename": "CHANGELOG.md", "status": "added"},
				map[string]any{"filename": "changelog.d/5-go-validator.md", "status": "removed"},
				map[string]any{
					"filename":          "changelog.d/3-repo-ops-v0.1.0.md",
					"status":            status,
					"previous_filename": "changelog.d/11-changelog-machine-record.md",
				},
			}
			contents := map[string]any{
				"changelog.d/3-repo-ops-v0.1.0.md": "## Fixed\n- Consume a legacy trusted-base fragment.\n",
			}
			state := withChangelogLine(t, "repo-ops.changelog.v1 kind:release value:v0.1.0", files, contents, "")
			got := findings(t, state, CheckContract)
			if !containsFinding(got, "release changelog declarations must consume, not modify, fragments") {
				t.Fatalf("expected consume-not-modify rejection, got %v", got)
			}
			if !containsFinding(got, "fragment filename must use <issue>-<slug>.md or direct-<slug>.md") {
				t.Fatalf("expected incoming filename rejection, got %v", got)
			}
		})
	}
}

func TestReleaseCopyOutCannotSatisfyFragmentConsumption(t *testing.T) {
	files := []any{
		map[string]any{"filename": "CHANGELOG.md", "status": "added"},
		map[string]any{
			"filename":          "docs/copied-note.md",
			"status":            "copied",
			"previous_filename": "changelog.d/42-validator.md",
		},
	}
	state := withChangelogLine(t, "repo-ops.changelog.v1 kind:release value:v0.1.0", files, nil, "")
	got := findings(t, state, CheckContract)
	if !containsFinding(got, "release changelog declarations must consume at least one fragment") {
		t.Fatalf("copy out of changelog.d must not count as consumption, got %v", got)
	}
}

func TestRequiredDeclarationAcceptsCopiedOwnedFragment(t *testing.T) {
	files := []any{
		map[string]any{
			"filename":          "changelog.d/42-validator.md",
			"status":            "copied",
			"previous_filename": "changelog.d/42-source.md",
		},
	}
	contents := map[string]any{
		"changelog.d/42-validator.md": "## Changed\n- Validate the trusted policy contract.\n",
	}
	state := withChangelogLine(t, "repo-ops.changelog.v1 kind:required value:fragment-added", files, contents, "")
	got := findings(t, state, CheckContract)
	if len(got) != 0 {
		t.Fatalf("owned copied fragment should pass, got %v", got)
	}
}

func TestOrdinaryPRRejectsRenamedChangelogPrevious(t *testing.T) {
	files := []any{
		map[string]any{
			"filename":          "docs/history.md",
			"status":            "renamed",
			"previous_filename": "CHANGELOG.md",
		},
	}
	state := withChangelogLine(t, fixtureChangelog, files, nil, "")
	got := findings(t, state, CheckContract)
	if !containsFinding(got, "ordinary pull requests must not edit CHANGELOG.md") {
		t.Fatalf("expected CHANGELOG rename rejection, got %v", got)
	}
}

func TestUnsupportedStatusCannotSatisfyRequiredOrRelease(t *testing.T) {
	t.Run("required", func(t *testing.T) {
		files := []any{
			map[string]any{"filename": "changelog.d/42-validator.md", "status": "unchanged"},
		}
		contents := map[string]any{
			"changelog.d/42-validator.md": "## Changed\n- Validate the trusted policy contract.\n",
		}
		state := withChangelogLine(t, "repo-ops.changelog.v1 kind:required value:fragment-added", files, contents, "")
		got := findings(t, state, CheckContract)
		if !containsFinding(got, "changelog file status must be added, modified, renamed, copied, or removed") {
			t.Fatalf("expected unsupported status finding, got %v", got)
		}
		if !containsFinding(got, "required changelog declarations need an added or modified fragment") {
			t.Fatalf("unchanged must not satisfy required, got %v", got)
		}
	})
	t.Run("release", func(t *testing.T) {
		files := []any{
			map[string]any{"filename": "CHANGELOG.md", "status": "unchanged"},
			map[string]any{"filename": "changelog.d/42-validator.md", "status": "removed"},
		}
		state := withChangelogLine(t, "repo-ops.changelog.v1 kind:release value:v0.1.0", files, nil, "")
		got := findings(t, state, CheckContract)
		if !containsFinding(got, "changelog file status must be added, modified, renamed, copied, or removed") {
			t.Fatalf("expected unsupported status finding, got %v", got)
		}
		if !containsFinding(got, "release changelog declarations must update CHANGELOG.md") {
			t.Fatalf("unchanged must not satisfy release, got %v", got)
		}
	})
}

func TestChangelogRecordAcceptsIndentation(t *testing.T) {
	state := withChangelogLine(t, "    "+fixtureChangelog, nil, nil, "")
	if got := findings(t, state, CheckContract); len(got) != 0 {
		t.Fatalf("indented v1 record should pass, got %v", got)
	}
}

func TestFencedChangelogExamplesAreNotRecords(t *testing.T) {
	fenced := "```text\nrepo-ops.changelog.v1 kind:required value:fragment-added\n```"
	got := findings(t, withChangelogLine(t, fenced, changelogBait(), nil, ""), CheckContract)
	if !containsFinding(got, exactlyOneChangelog) {
		t.Fatalf("fenced-only body should be missing, got %v", got)
	}
	assertNoChangelogLifecycle(t, got)

	state := withChangelogLine(t, fixtureChangelog, nil, nil, "\n"+fenced+"\n")
	if got := findings(t, state, CheckContract); len(got) != 0 {
		t.Fatalf("fenced example beside a valid record should pass, got %v", got)
	}

	for _, closer := range []string{"```\u00a0", "```\u2028"} {
		deceptive := "```\n" + closer + "\n" + fixtureChangelog + "\n"
		got := findings(t, withChangelogLine(t, deceptive, nil, nil, ""), CheckContract)
		if !containsFinding(got, exactlyOneChangelog) {
			t.Fatalf("deceptive closer %q should keep the record fenced, got %v", closer, got)
		}
		assertNoChangelogLifecycle(t, got)
	}
}

func TestLegacyChangelogLineIsNonAuthoritative(t *testing.T) {
	legacy := "- Changelog: required — users need the release note"
	got := findings(t, withChangelogLine(t, legacy, changelogBait(), nil, ""), CheckContract)
	if !containsFinding(got, exactlyOneChangelog) {
		t.Fatalf("legacy-only should be missing, got %v", got)
	}
	assertNoChangelogLifecycle(t, got)

	state := withChangelogLine(t, fixtureChangelog, nil, nil, "\n"+legacy+"\n")
	if got := findings(t, state, CheckContract); len(got) != 0 {
		t.Fatalf("legacy-plus-valid should pass, got %v", got)
	}
}

func TestChangelogCandidateCountPrecedesLifecycle(t *testing.T) {
	missing := findings(t, withChangelogLine(t, "", changelogBait(), nil, ""), CheckContract)
	duplicate := findings(t, withChangelogLine(t,
		"repo-ops.changelog.v1 kind:required value:fragment-added\n"+
			"repo-ops.changelog.v1 kind:required value:fragment-added",
		nil, nil, ""), CheckContract)
	validPlusMalformed := findings(t, withChangelogLine(t,
		"repo-ops.changelog.v1 kind:required value:fragment-added\n"+
			"repo-ops.changelog.v1 kind:required",
		nil, nil, ""), CheckContract)
	for _, got := range [][]string{missing, duplicate, validPlusMalformed} {
		if !containsFinding(got, exactlyOneChangelog) {
			t.Fatalf("expected count finding, got %v", got)
		}
		if containsFinding(got, malformedChangelog) {
			t.Fatalf("count must precede malformed, got %v", got)
		}
		assertNoChangelogLifecycle(t, got)
	}
}

func TestMalformedChangelogRecordsPrecedeLifecycle(t *testing.T) {
	lines := []string{
		"repo-ops.changelog.v1 value:fragment-added kind:required",
		"repo-ops.changelog.v1 kind:required",
		"repo-ops.changelog.v1 kind:required value:fragment-added kind:required",
		"repo-ops.changelog.v1 kind:required value:fragment-added extra:token",
		"repo-ops.changelog.v1\tkind:required value:fragment-added",
		"repo-ops.changelog.v1  kind:required value:fragment-added",
		"repo-ops.changelog.v1 kind:required value:fragment-added leftover",
	}
	for _, line := range lines {
		got := findings(t, withChangelogLine(t, line, changelogBait(), nil, ""), CheckContract)
		if !containsFinding(got, malformedChangelog) {
			t.Fatalf("%q: expected malformed, got %v", line, got)
		}
		if containsFinding(got, exactlyOneChangelog) {
			t.Fatalf("%q: malformed must not be a count finding, got %v", line, got)
		}
		assertNoChangelogLifecycle(t, got)
	}
}

func TestChangelogSchemaIdentityPrecedesLifecycle(t *testing.T) {
	lines := []string{
		"repo-ops.changelog.v1 kind:optional value:fragment-added",
		"repo-ops.changelog.v1 kind:required value:Not_Valid",
		"repo-ops.changelog.v1 kind:not-required value:replace-me",
		"repo-ops.changelog.v1 kind:release value:v01.0.0",
		"repo-ops.changelog.v1 kind:release value:v0.1.0-rc.1",
	}
	for _, line := range lines {
		got := findings(t, withChangelogLine(t, line, changelogBait(), nil, ""), CheckContract)
		if !containsFinding(got, changelogSchemaLabel) {
			t.Fatalf("%q: expected schema identity, got %v", line, got)
		}
		if containsFinding(got, exactlyOneChangelog) {
			t.Fatalf("%q: schema must not be a count finding, got %v", line, got)
		}
		if containsFinding(got, malformedChangelog) {
			t.Fatalf("%q: schema must not be a malformed finding, got %v", line, got)
		}
		assertNoChangelogLifecycle(t, got)
	}
}

func TestUntrustedRecordsAreIgnored(t *testing.T) {
	state := cloneState(t, loadJSON(t, "valid-low-direct.json"))
	comments := asEntries(state["comments"])
	state["comments"] = []any{
		map[string]any{"author": "external", "association": "NONE", "body": "repo-ops.merge-review.v1 malformed"},
		comments[0],
	}
	if got := findings(t, state, CheckAll); len(got) != 0 {
		t.Fatalf("untrusted malformed comment must be ignored, got %v", got)
	}
}

func TestSharedIdentityRequiresNativeReview(t *testing.T) {
	state := cloneState(t, loadJSON(t, "valid-low-direct.json"))
	asMap(asMap(state["policy"])["review"])["shared_identity"] = false
	commentAt(state, 0)["author"] = "review-bot"
	got := findings(t, state, CheckAll)
	if !containsFinding(got, "independent review receipt must be submitted as a native GitHub Review") {
		t.Fatalf("expected native-review finding, got %v", got)
	}
}

func TestRecordRejectsNoncanonicalSpacing(t *testing.T) {
	orders := map[string][]string{mergeReviewRecord: {"verdict", "risk", "intent", "plan", "base", "head", "diff", "runtime", "model"}}
	_, errMsg := parseRecordLine("repo-ops.merge-review.v1  verdict:merge-ready", mergeReviewRecord, orders)
	if errMsg != "repo-ops.merge-review.v1 fields must use one ASCII space" {
		t.Fatalf("got %q", errMsg)
	}
}

func TestFencedIntentExamplesAreNotAuthority(t *testing.T) {
	examples := []string{
		"```text\nrepo-ops.intent.v1 decision:accepted risk:high intent:sha256:<digest> supersedes:<previous>\n```\n",
		"```text\nIntent accepted.\nIntent digest: sha256:<digest>\n```\n",
	}
	for _, body := range examples {
		parsed, errMsg := parseIntent(body, 42, contractRoot(t), map[string][]string{
			intentRecord: {"decision", "risk", "intent", "supersedes"},
		})
		if parsed != nil || errMsg != "" {
			t.Fatalf("fenced example became authority: parsed=%v err=%q", parsed, errMsg)
		}
	}
}

func TestStaleHeadIsMergeBlocked(t *testing.T) {
	state := cloneState(t, loadJSON(t, "valid-low-direct.json"))
	commentAt(state, 0)["body"] = strings.Replace(
		asString(commentAt(state, 0)["body"]),
		"head:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"head:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		1,
	)
	got := findings(t, state, CheckAll)
	if !containsFinding(got, "merge-review head does not match the current pull request") {
		t.Fatalf("expected stale head finding, got %v", got)
	}
	if containsFinding(findings(t, state, CheckContract), "merge-review head does not match the current pull request") {
		t.Fatal("contract check must not include merge receipt")
	}
}

func TestAuthorityIgnoresFencedDirectRoute(t *testing.T) {
	state := v1State(t)
	pr := asMap(state["pull_request"])
	pr["body"] = asString(pr["body"]) + "\n```markdown\nDirect low-risk PR: documentation-only update\n```\n"
	got := findings(t, state, CheckAll)
	if len(got) != 0 {
		t.Fatalf("fenced direct-route example must not change authority, got %v", got)
	}
}

func TestMissingPolicyEarlyReturn(t *testing.T) {
	got := ValidateState(map[string]any{}, contractRoot(t), CheckAll)
	if len(got) != 1 || got[0] != "repository policy is missing or invalid YAML" {
		t.Fatalf("got %v", got)
	}
}

func TestSchemaFailureMappingIsStructured(t *testing.T) {
	state := cloneState(t, loadJSON(t, "valid-low-direct.json"))
	asMap(state["policy"])["contract"] = 1
	got := findings(t, state, CheckContract)
	if !containsFinding(got, "repository policy:") {
		t.Fatalf("expected mapped policy schema finding, got %v", got)
	}
	for _, finding := range got {
		if strings.Contains(finding, "repository policy:") && (strings.Contains(finding, "const") || strings.Contains(finding, "/contract")) {
			return
		}
	}
	t.Fatalf("schema finding should carry path/keyword identity, got %v", got)
}
