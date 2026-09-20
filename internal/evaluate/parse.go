package evaluate

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/haesol-shin/.github/internal/canonical"
	"github.com/haesol-shin/.github/internal/schema"
)

const (
	intentRecord       = "repo-ops.intent.v1"
	planApprovalRecord = "repo-ops.plan-approval.v1"
	mergeReviewRecord  = "repo-ops.merge-review.v1"
)

var (
	authorizedAssociations = []string{"OWNER"}
	repositoryPermissions  = []string{"maintain", "admin"}

	riskPattern         = regexp.MustCompile(`(?m)^Risk:\s*(low|medium|high)\s+(?:—|-)\s+\S`)
	directPattern       = regexp.MustCompile(`(?i)^\s*Direct low-risk PR:\s*\S`)
	planPathPattern     = regexp.MustCompile(`(?mi)^Plan:\s*(.+?)\s*$`)
	markdownLinkPattern = regexp.MustCompile(`\A\[[^]]+\]\(([^)]+)\)\z`)
	blobPathPattern     = regexp.MustCompile(`/blob/[^/]+/(.+)$`)

	v1AcceptedMarker   = regexp.MustCompile(`(?m)^\*\*Intent accepted\*\*\s*$`)
	v1RecordMarker     = regexp.MustCompile(`(?m)^\s*repo-ops\.intent\.v1\b`)
	legacyIntentMarker = regexp.MustCompile(`(?m)^Intent accepted\.\s*$`)

	v1IntentWrapper = regexp.MustCompile(
		`(?s)\A\*\*Intent accepted\*\*\n\n` +
			`(?P<outcome>.+?)\n\n` +
			`- Risk: ` + "`" + `(?P<display_risk>low|medium|high)` + "`" + `\n` +
			`- Intent digest: ` + "`" + `(?P<display_digest>sha256:[0-9a-f]{64})` + "`" + `\n` +
			`- Supersedes: ` + "`" + `(?P<display_supersedes>none|sha256:[0-9a-f]{64})` + "`" + `\n\n` +
			`Authoritative machine-readable record:\n\n` +
			"```text\n(?:repo-ops\\.intent\\.v1[^\\n]*)\n```\n" +
			`\z`,
	)
	legacyIntentWrapper = regexp.MustCompile(
		`(?s)\AIntent accepted\.\n\n(?P<outcome>.+?)\n\n` +
			`Risk: (?P<risk>low|medium|high)\n` +
			`Intent digest: (?P<digest>sha256:[0-9a-f]{64})\n` +
			`\z`,
	)
	planApprovalWrapper = regexp.MustCompile(
		`\A\*\*Plan approved(?: — [^\n]+)?\*\*\n\n` +
			`- Risk: ` + "`" + `(?P<display_risk>low|medium|high)` + "`" + `\n` +
			`- Intent digest: ` + "`" + `(?P<display_intent>sha256:[0-9a-f]{64})` + "`" + `\n` +
			`- Plan digest: ` + "`" + `(?P<display_plan>sha256:[0-9a-f]{64})` + "`" + `\n` +
			`- Plan commit: ` + "`" + `(?P<display_commit>[0-9a-f]{40})` + "`" + `\n\n` +
			`Authoritative machine-readable record:\n\n` +
			"```text\n" +
			`repo-ops\.plan-approval\.v1[^\n]*\n` +
			"```\n" +
			`\z`,
	)
)

func recordCandidate(entry map[string]any) bool {
	return inSet(asString(entry["association"]), authorizedAssociations) ||
		inSet(asString(entry["permission"]), repositoryPermissions)
}

func recordAuthorized(entry map[string]any, record string, risk string, authority map[string]riskAuthority) bool {
	if record == planApprovalRecord {
		if risk == "" {
			return false
		}
		spec := authority[risk]
		return inSet(asString(entry["association"]), spec.ApprovalAssociations) ||
			inSet(asString(entry["permission"]), spec.ApprovalPermissions)
	}
	return asString(entry["association"]) == "OWNER" ||
		inSet(asString(entry["permission"]), repositoryPermissions)
}

func parseRisk(body string) string {
	match := riskPattern.FindStringSubmatch(body)
	if match == nil {
		return ""
	}
	return match[1]
}

func parseAuthorityLinks(body string, keywords []string, exactlyOne bool) (links []int, errors []string) {
	if len(keywords) == 0 {
		keywords = []string{"Fixes", "Closes", "Related"}
	}
	escaped := make([]string, len(keywords))
	for i, keyword := range keywords {
		escaped[i] = regexp.QuoteMeta(keyword)
	}
	keywordPattern := strings.Join(escaped, "|")
	validPattern := regexp.MustCompile(`^(?:` + keywordPattern + `)[ \t]+#(?P<issue>[1-9][0-9]*)$`)
	candidatePattern := regexp.MustCompile(`^(?:` + keywordPattern + `)[ \t]*#`)
	validPattern = caseInsensitive(validPattern)
	candidatePattern = caseInsensitive(candidatePattern)

	for _, line := range markdownBlockLines(body) {
		stripped := strings.TrimSpace(line)
		if !candidatePattern.MatchString(stripped) {
			continue
		}
		match := validPattern.FindStringSubmatch(stripped)
		if match == nil || !validPattern.MatchString(stripped) {
			errors = append(errors, "issue authority lines must use `Fixes|Closes|Related #<issue>`")
			continue
		}
		issue, _ := strconv.Atoi(match[validPattern.SubexpIndex("issue")])
		links = append(links, issue)
	}
	if exactlyOne && len(links) > 1 {
		errors = append(errors, "pull request must contain exactly one standalone issue authority line")
	}
	return links, errors
}

func caseInsensitive(re *regexp.Regexp) *regexp.Regexp {
	return regexp.MustCompile(`(?i)` + re.String())
}

func parseRelatedIssue(body string, keywords []string, exactlyOne bool) *int {
	links, _ := parseAuthorityLinks(body, keywords, exactlyOne)
	if len(links) != 1 {
		return nil
	}
	issue := links[0]
	return &issue
}

func isDirectLowRisk(body string) bool {
	for _, line := range markdownBlockLines(body) {
		if directPattern.MatchString(line) {
			return true
		}
	}
	return false
}

func parsePlanPath(body string) string {
	match := planPathPattern.FindStringSubmatch(body)
	if match == nil {
		return ""
	}
	value := strings.Trim(strings.TrimSpace(match[1]), "`")
	if strings.EqualFold(value, "none") {
		return ""
	}
	if markdownLink := markdownLinkPattern.FindStringSubmatch(value); markdownLink != nil {
		value = markdownLink[1]
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		parsed, err := url.Parse(value)
		if err != nil {
			return ""
		}
		blob := blobPathPattern.FindStringSubmatch(parsed.Path)
		if blob == nil {
			return ""
		}
		value = blob[1]
	}
	value = strings.TrimPrefix(value, "./")
	if strings.HasPrefix(value, ".ops/plans/") {
		return value
	}
	return ""
}

func parseRecordLine(body, record string, orders map[string][]string) (map[string]any, string) {
	var matching []string
	for _, line := range splitLines(body) {
		stripped := strings.TrimSpace(line)
		if strings.HasPrefix(stripped, record) {
			matching = append(matching, stripped)
		}
	}
	if len(matching) == 0 {
		return nil, ""
	}
	if len(matching) > 1 {
		return nil, record + " must appear exactly once in one comment"
	}
	tokens := strings.Split(matching[0], " ")
	for _, token := range tokens {
		if token == "" {
			return nil, record + " fields must use one ASCII space"
		}
	}
	if tokens[0] != record {
		return nil, "malformed " + record + " identifier"
	}
	values := map[string]any{"record": record}
	keys := make([]string, 0, len(tokens)-1)
	for _, token := range tokens[1:] {
		key, value, ok := strings.Cut(token, ":")
		if !ok {
			return nil, "malformed " + record + " field: " + token
		}
		if key == "" || value == "" {
			return nil, "malformed or duplicate " + record + " field: " + token
		}
		if _, exists := values[key]; exists {
			return nil, "malformed or duplicate " + record + " field: " + token
		}
		keys = append(keys, key)
		if key == "issue" && isASCIIDigits(value) {
			n, _ := strconv.Atoi(value)
			values[key] = n
		} else {
			values[key] = value
		}
	}
	expected := orders[record]
	if !stringSliceEqual(keys, expected) {
		return nil, record + " fields must appear in this order: " + strings.Join(expected, " ")
	}
	return values, ""
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func parseIntent(body string, issue any, contractRoot string, orders map[string][]string) (map[string]any, string) {
	normalized := canonical.NormalizeText(body)
	unfenced := strings.Join(markdownBlockLines(normalized), "\n")
	hasV1 := v1AcceptedMarker.MatchString(unfenced) || v1RecordMarker.MatchString(unfenced)
	if hasV1 {
		match := v1IntentWrapper.FindStringSubmatch(normalized)
		if match == nil {
			return nil, "malformed repo-ops.intent.v1 accepted-intent comment"
		}
		record, recordError := parseRecordLine(body, intentRecord, orders)
		if recordError != "" {
			return nil, recordError
		}
		if record == nil {
			return nil, "repo-ops.intent.v1 record is missing"
		}
		schemaErrors := schemaFindings(record, filepath.Join(contractRoot, "intent.schema.json"), intentRecord)
		if len(schemaErrors) > 0 {
			return nil, schemaErrors[0]
		}
		issueN, ok := asInt(issue)
		if issue == nil || !ok || issueN < 1 {
			return nil, "repo-ops.intent.v1 requires a positive linked issue"
		}
		outcome := match[v1IntentWrapper.SubexpIndex("outcome")]
		computed, err := canonical.CanonicalDigest(map[string]any{
			"issue":   issueN,
			"outcome": canonical.NormalizeText(outcome),
			"risk":    record["risk"],
		})
		if err != nil {
			return nil, "accepted-intent digest does not match its canonical issue-bound payload"
		}
		if match[v1IntentWrapper.SubexpIndex("display_risk")] != asString(record["risk"]) {
			return nil, "accepted-intent displayed risk does not match its raw record"
		}
		if match[v1IntentWrapper.SubexpIndex("display_digest")] != asString(record["intent"]) {
			return nil, "accepted-intent displayed digest does not match its raw record"
		}
		if match[v1IntentWrapper.SubexpIndex("display_supersedes")] != asString(record["supersedes"]) {
			return nil, "accepted-intent displayed supersession does not match its raw record"
		}
		if asString(record["intent"]) != computed {
			return nil, "accepted-intent digest does not match its canonical issue-bound payload"
		}
		return map[string]any{
			"kind":       "v1",
			"digest":     computed,
			"risk":       record["risk"],
			"supersedes": record["supersedes"],
			"outcome":    outcome,
		}, ""
	}
	if !legacyIntentMarker.MatchString(unfenced) {
		return nil, ""
	}
	match := legacyIntentWrapper.FindStringSubmatch(normalized)
	if match == nil {
		return nil, "malformed legacy accepted-intent comment"
	}
	outcome := match[legacyIntentWrapper.SubexpIndex("outcome")]
	risk := match[legacyIntentWrapper.SubexpIndex("risk")]
	computed, err := canonical.CanonicalDigest(map[string]any{
		"outcome": canonical.NormalizeText(outcome),
		"risk":    risk,
	})
	if err != nil || computed != match[legacyIntentWrapper.SubexpIndex("digest")] {
		return nil, "accepted-intent digest does not match its canonical payload"
	}
	return map[string]any{
		"kind":    "legacy",
		"digest":  computed,
		"risk":    risk,
		"outcome": outcome,
	}, ""
}

func parsePlanApproval(body string, orders map[string][]string) (map[string]any, string) {
	normalized := canonical.NormalizeText(body)
	match := planApprovalWrapper.FindStringSubmatch(normalized)
	if match == nil {
		return nil, "malformed repo-ops.plan-approval.v1 display wrapper"
	}
	record, recordError := parseRecordLine(body, planApprovalRecord, orders)
	if recordError != "" {
		return nil, recordError
	}
	if record == nil {
		return nil, "repo-ops.plan-approval.v1 record is missing"
	}
	displayed := []struct {
		field string
		value string
	}{
		{"risk", match[planApprovalWrapper.SubexpIndex("display_risk")]},
		{"intent", match[planApprovalWrapper.SubexpIndex("display_intent")]},
		{"plan", match[planApprovalWrapper.SubexpIndex("display_plan")]},
		{"plan-commit", match[planApprovalWrapper.SubexpIndex("display_commit")]},
	}
	for _, item := range displayed {
		if asString(record[item.field]) != item.value {
			return nil, "plan approval displayed " + item.field + " does not match its raw record"
		}
	}
	return record, ""
}

func latestRecord(entries []map[string]any, record string, orders map[string][]string) (map[string]any, map[string]any, []string) {
	var found [][2]map[string]any
	var errors []string
	for _, entry := range entries {
		body := orEmptyString(entry["body"])
		if !strings.Contains(body, record) {
			continue
		}
		var parsed map[string]any
		var errMsg string
		if record == planApprovalRecord {
			parsed, errMsg = parsePlanApproval(body, orders)
		} else {
			parsed, errMsg = parseRecordLine(body, record, orders)
		}
		if errMsg != "" {
			errors = append(errors, errMsg)
		} else if parsed != nil {
			found = append(found, [2]map[string]any{parsed, entry})
		}
	}
	if len(found) == 0 {
		return nil, nil, errors
	}
	last := found[len(found)-1]
	return last[0], last[1], errors
}

func schemaFindings(instance any, schemaPath, label string) []string {
	failures, err := schema.ValidateFile(instance, schemaPath, label)
	if err != nil {
		return []string{label + ": schema evaluation failed"}
	}
	out := make([]string, 0, len(failures))
	for _, failure := range failures {
		out = append(out, formatSchemaFailure(failure))
	}
	return out
}

func formatSchemaFailure(failure schema.Failure) string {
	path := failure.InstancePath
	if path == "" {
		path = "/"
	}
	keyword := failure.Keyword
	if keyword == "" {
		keyword = "valid"
	}
	return failure.Label + ": " + path + " " + keyword
}
