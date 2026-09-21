package collect

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/haesol-shin/.github/internal/canonical"
)

const planApprovalRecord = "repo-ops.plan-approval.v1"

var (
	authorizedAssociations = []string{"OWNER"}
	repositoryPermissions  = []string{"maintain", "admin"}
	planApprovalOrder      = []string{"decision", "risk", "issue", "intent", "plan", "plan-commit"}
	authorityKeywords      = []string{"Fixes", "Closes", "Related"}

	planPathPattern     = regexp.MustCompile(`(?mi)^Plan:\s*(.+?)\s*$`)
	markdownLinkPattern = regexp.MustCompile(`\A\[[^]]+\]\(([^)]+)\)\z`)
	blobPathPattern     = regexp.MustCompile(`/blob/[^/]+/(.+)$`)
	fenceLinePattern    = regexp.MustCompile("^ {0,3}([`]{3,}|~{3,})(.*)$")
	blockLinePattern    = regexp.MustCompile(`^ {0,3}\S`)
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

func fragmentPath(filename, root string) bool {
	normalized := strings.ReplaceAll(filename, "\\", "/")
	prefix := strings.Trim(strings.ReplaceAll(root, "\\", "/"), "/") + "/"
	if !strings.HasPrefix(normalized, prefix) {
		return false
	}
	relative := normalized[len(prefix):]
	return !strings.Contains(relative, "/") && strings.HasSuffix(relative, ".md") && relative != "README.md"
}

func recordCandidate(entry map[string]any) bool {
	return inSet(asString(entry["association"]), authorizedAssociations) ||
		inSet(asString(entry["permission"]), repositoryPermissions)
}

func parseRelatedIssue(body string) any {
	links, _ := parseAuthorityLinks(body)
	if len(links) != 1 {
		return nil
	}
	return links[0]
}

func parseAuthorityLinks(body string) ([]int, []string) {
	escaped := make([]string, len(authorityKeywords))
	for i, keyword := range authorityKeywords {
		escaped[i] = regexp.QuoteMeta(keyword)
	}
	keywordPattern := strings.Join(escaped, "|")
	validPattern := regexp.MustCompile(`(?i)^(?:` + keywordPattern + `)[ \t]+#(?P<issue>[1-9][0-9]*)$`)
	candidatePattern := regexp.MustCompile(`(?i)^(?:` + keywordPattern + `)[ \t]*#`)
	var links []int
	var errors []string
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
	if len(links) > 1 {
		errors = append(errors, "pull request must contain exactly one standalone issue authority line")
	}
	return links, errors
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

func parsePlanApproval(body string) map[string]any {
	normalized := canonical.NormalizeText(body)
	match := planApprovalWrapper.FindStringSubmatch(normalized)
	if match == nil {
		return nil
	}
	record, errMsg := parseRecordLine(body, planApprovalRecord, planApprovalOrder)
	if errMsg != "" || record == nil {
		return nil
	}
	displayed := []struct{ field, value string }{
		{"risk", match[planApprovalWrapper.SubexpIndex("display_risk")]},
		{"intent", match[planApprovalWrapper.SubexpIndex("display_intent")]},
		{"plan", match[planApprovalWrapper.SubexpIndex("display_plan")]},
		{"plan-commit", match[planApprovalWrapper.SubexpIndex("display_commit")]},
	}
	for _, item := range displayed {
		if asString(record[item.field]) != item.value {
			return nil
		}
	}
	return record
}

func parseRecordLine(body, record string, order []string) (map[string]any, string) {
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
		if !ok || key == "" || value == "" {
			if !ok {
				return nil, "malformed " + record + " field: " + token
			}
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
	if len(keys) != len(order) {
		return nil, record + " fields must appear in this order: " + strings.Join(order, " ")
	}
	for i := range keys {
		if keys[i] != order[i] {
			return nil, record + " fields must appear in this order: " + strings.Join(order, " ")
		}
	}
	return values, ""
}

func splitLines(body string) []string {
	lines := make([]string, 0, 16)
	start := 0
	for i := 0; i < len(body); {
		if body[i] == '\r' {
			lines = append(lines, body[start:i])
			i++
			if i < len(body) && body[i] == '\n' {
				i++
			}
			start = i
			continue
		}
		if body[i] == '\n' {
			lines = append(lines, body[start:i])
			i++
			start = i
			continue
		}
		i++
	}
	return append(lines, body[start:])
}

func unfencedLines(body string) []string {
	var (
		out            []string
		fenceCharacter byte
		fenceLength    int
	)
	for _, line := range splitLines(body) {
		fenceMatch := fenceLinePattern.FindStringSubmatch(line)
		if fenceCharacter != 0 {
			if fenceMatch != nil {
				marker := fenceMatch[1]
				if marker[0] == fenceCharacter && len(marker) >= fenceLength && fenceRemainderIsSpaceTab(fenceMatch[2]) {
					fenceCharacter = 0
					fenceLength = 0
				}
			}
			continue
		}
		if fenceMatch != nil {
			marker := fenceMatch[1]
			if marker[0] == '`' && strings.Contains(fenceMatch[2], "`") {
				out = append(out, line)
				continue
			}
			fenceCharacter = marker[0]
			fenceLength = len(marker)
			continue
		}
		out = append(out, line)
	}
	return out
}

func fenceRemainderIsSpaceTab(rest string) bool {
	for i := range rest {
		if rest[i] != ' ' && rest[i] != '\t' {
			return false
		}
	}
	return true
}

func markdownBlockLines(body string) []string {
	var out []string
	for _, line := range unfencedLines(body) {
		if blockLinePattern.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}

func isASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func inSet(value string, set []string) bool {
	for _, item := range set {
		if item == value {
			return true
		}
	}
	return false
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func lookupInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), int64(int(n)) == n
	case float64:
		i := int(n)
		return i, float64(i) == n
	default:
		return 0, false
	}
}

func nestedString(m map[string]any, keys ...string) (string, bool) {
	var cur any = m
	for _, key := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return "", false
		}
		cur = obj[key]
	}
	s, ok := cur.(string)
	return s, ok
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func orEmptyString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
