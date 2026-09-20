package evaluate

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Check selects which evaluator stages run. CheckContract omits merge receipt
// and quality. CheckAll and CheckMergeApproval include them.
type Check string

const (
	CheckAll           Check = "all"
	CheckContract      Check = "contract"
	CheckMergeApproval Check = "merge-approval"
)

type riskAuthority struct {
	ApprovalAssociations []string
	ApprovalPermissions  []string
}

type loadedContract struct {
	ID            string
	RecordOrders  map[string][]string
	RiskAuthority map[string]riskAuthority
	Keywords      []string
	ExactlyOne    bool
	PullRequest   map[string]any
	Issue         map[string]any
}

type contractJSON struct {
	ID      string         `json:"id"`
	Issue   map[string]any `json:"issue"`
	Records map[string]struct {
		Schema     string   `json:"schema"`
		FieldOrder []string `json:"field_order"`
	} `json:"records"`
	RiskAuthority map[string]struct {
		Plan                 string   `json:"plan"`
		ApprovalAssociations []string `json:"approval_associations"`
		ApprovalPermissions  []string `json:"approval_permissions"`
	} `json:"risk_authority"`
	PullRequest map[string]any `json:"pull_request"`
}

func loadContract(contractRoot string) (loadedContract, error) {
	raw, err := os.ReadFile(filepath.Join(contractRoot, "contract.json"))
	if err != nil {
		return loadedContract{}, err
	}
	var doc contractJSON
	if err := json.Unmarshal(raw, &doc); err != nil {
		return loadedContract{}, err
	}
	orders := make(map[string][]string, len(doc.Records))
	for name, def := range doc.Records {
		orders[name] = def.FieldOrder
	}
	authority := make(map[string]riskAuthority, len(doc.RiskAuthority))
	for risk, spec := range doc.RiskAuthority {
		authority[risk] = riskAuthority{
			ApprovalAssociations: spec.ApprovalAssociations,
			ApprovalPermissions:  spec.ApprovalPermissions,
		}
	}
	keywords := []string{"Fixes", "Closes", "Related"}
	exactlyOne := true
	if doc.PullRequest != nil {
		if links := asMap(doc.PullRequest["authority_links"]); links != nil {
			if ks := stringList(links["keywords"]); len(ks) > 0 {
				keywords = ks
			}
			if v, ok := asBool(links["exactly_one"]); ok {
				exactlyOne = v
			}
		}
	}
	return loadedContract{
		ID:            doc.ID,
		RecordOrders:  orders,
		RiskAuthority: authority,
		Keywords:      keywords,
		ExactlyOne:    exactlyOne,
		PullRequest:   doc.PullRequest,
		Issue:         doc.Issue,
	}, nil
}

// ValidateState evaluates fixture-mode repository policy. Finding order matches
// Python validate_state append order. Schema failures are mapped to
// label/path/keyword identity rather than Python jsonschema wording.
func ValidateState(state map[string]any, contractRoot string, check Check) []string {
	if state == nil {
		state = map[string]any{}
	}
	if check == "" {
		check = CheckAll
	}
	contract, err := loadContract(contractRoot)
	if err != nil {
		return []string{"repository policy contract is unavailable"}
	}

	policy := asMap(state["policy"])
	if policy == nil {
		return []string{"repository policy is missing or invalid YAML"}
	}

	var errors []string
	errors = append(errors, schemaFindings(policy, filepath.Join(contractRoot, "repository-policy.schema.json"), "repository policy")...)
	if policy["contract"] != contract.ID {
		errors = append(errors, "repository policy must adopt "+contract.ID)
	}

	pullRequest := asMap(state["pull_request"])
	if pullRequest == nil {
		pullRequest = map[string]any{}
	}
	body := orEmptyString(pullRequest["body"])
	errors = append(errors, validateHeadingContract(body, contract.PullRequest, "pull request body")...)

	risk := parseRisk(body)
	if risk == "" {
		errors = append(errors, "pull request body must contain `Risk: low|medium|high — rationale`")
		return errors
	}

	issue := state["issue"]
	authorityLinks, authorityErrors := parseAuthorityLinks(body, contract.Keywords, contract.ExactlyOne)
	errors = append(errors, authorityErrors...)
	direct := isDirectLowRisk(body)
	if direct {
		if risk != "low" {
			errors = append(errors, "only low-risk work may use the direct pull request route")
		}
		if len(authorityLinks) > 0 {
			errors = append(errors, "direct low-risk work must not include issue authority")
		}
		if issue != nil {
			errors = append(errors, "direct low-risk work must not link an issue")
		}
	} else if len(authorityErrors) == 0 {
		if len(authorityLinks) != 1 {
			errors = append(errors, "non-direct work must link an issue with `Fixes #<issue>` "+
				"(or `Closes #<issue>` or `Related #<issue>`)")
		} else if issue == nil {
			errors = append(errors, "issue authority requires a linked issue")
		} else if !sameValue(authorityLinks[0], issue) {
			errors = append(errors, "issue authority does not match the linked issue")
		}
	}

	declaration, declarationErrors := parseChangelogDeclaration(body, contractRoot, contract.RecordOrders)
	errors = append(errors, declarationErrors...)
	errors = append(errors, validateChangelogState(state, policy, issue, direct, declaration)...)

	comments := asEntries(state["comments"])
	if comments == nil {
		comments = []map[string]any{}
	}
	if issue != nil {
		issueBody := orEmptyString(state["issue_body"])
		errors = append(errors, validateHeadingContract(issueBody, contract.Issue, "linked issue body")...)
	}
	reviews := asEntries(state["reviews"])
	if reviews == nil {
		reviews = []map[string]any{}
	}

	var authorizedComments, authorizedReviews []map[string]any
	for _, entry := range comments {
		if recordCandidate(entry) {
			authorizedComments = append(authorizedComments, entry)
		}
	}
	for _, entry := range reviews {
		if recordCandidate(entry) {
			authorizedReviews = append(authorizedReviews, entry)
		}
	}
	records := make([]map[string]any, 0, len(authorizedComments)+len(authorizedReviews))
	for _, entry := range authorizedComments {
		records = append(records, copyEntry(entry, "comment"))
	}
	for _, entry := range authorizedReviews {
		records = append(records, copyEntry(entry, "review"))
	}

	expectedIntent := "none"
	if issue != nil {
		var chainHead map[string]any
		sawV1 := false
		for _, comment := range asEntries(state["intent_comments"]) {
			if !recordCandidate(comment) {
				continue
			}
			parsedIntent, intentError := parseIntent(orEmptyString(comment["body"]), issue, contractRoot, contract.RecordOrders)
			if intentError != "" {
				errors = append(errors, intentError)
				continue
			}
			if parsedIntent == nil {
				continue
			}
			if !recordAuthorized(comment, intentRecord, "", contract.RiskAuthority) {
				errors = append(errors, "accepted intent was not posted by an authorized maintainer")
				continue
			}
			if asString(parsedIntent["kind"]) == "legacy" {
				if sawV1 {
					errors = append(errors, "legacy accepted intent cannot follow a v1 intent record")
				} else {
					chainHead = parsedIntent
				}
				continue
			}
			sawV1 = true
			expectedSupersedes := "none"
			if chainHead != nil {
				expectedSupersedes = asString(chainHead["digest"])
			}
			if asString(parsedIntent["supersedes"]) != expectedSupersedes {
				errors = append(errors, "repo-ops.intent.v1 supersedes does not match the current intent")
				continue
			}
			chainHead = parsedIntent
		}
		if chainHead == nil {
			errors = append(errors, "linked issue has no valid maintainer accepted-intent comment")
		} else {
			expectedIntent = asString(chainHead["digest"])
			if asString(chainHead["risk"]) != risk {
				errors = append(errors, "pull request risk does not match accepted intent")
			}
		}
	}

	expectedPlan := orEmptyString(state["plan_digest"])
	if expectedPlan == "" {
		expectedPlan = "none"
	}
	if (risk == "medium" || risk == "high") && expectedPlan == "none" {
		errors = append(errors, risk+"-risk work requires a current plan")
	}

	var planComments []map[string]any
	for _, entry := range comments {
		if recordCandidate(entry) {
			planComments = append(planComments, entry)
		}
	}
	if risk == "medium" || risk == "high" {
		approval, approvalEntry, approvalErrors := latestRecord(planComments, planApprovalRecord, contract.RecordOrders)
		errors = append(errors, approvalErrors...)
		if approval == nil {
			errors = append(errors, risk+"-risk work requires a maintainer plan approval")
		} else {
			errors = append(errors, schemaFindings(approval, filepath.Join(contractRoot, "plan-approval.schema.json"), "plan approval")...)
			expectedApproval := map[string]any{
				"issue":  issue,
				"intent": expectedIntent,
				"plan":   expectedPlan,
			}
			for _, key := range []string{"issue", "intent", "plan"} {
				value := expectedApproval[key]
				if value != nil && !sameValue(approval[key], value) {
					errors = append(errors, "plan approval "+key+" does not match current work")
				}
			}
			if approvalEntry == nil || !recordAuthorized(approvalEntry, planApprovalRecord, risk, contract.RiskAuthority) {
				errors = append(errors, risk+"-risk plan approval was not posted by an authorized maintainer")
			}
			if !containsString(stringList(state["plan_commits"]), asString(approval["plan-commit"])) {
				errors = append(errors, "approved plan commit is not in the pull request history")
			}
		}
	}

	if check != CheckContract {
		receipt, receiptEntry, receiptErrors := latestRecord(records, mergeReviewRecord, contract.RecordOrders)
		errors = append(errors, receiptErrors...)
		if receipt == nil {
			errors = append(errors, "no merge-ready receipt was found")
		} else {
			errors = append(errors, schemaFindings(receipt, filepath.Join(contractRoot, "merge-review.schema.json"), "merge-review receipt")...)
			expected := map[string]any{
				"risk":   risk,
				"intent": expectedIntent,
				"plan":   expectedPlan,
				"base":   pullRequest["merge_base"],
				"head":   pullRequest["head"],
				"diff":   pullRequest["diff_digest"],
			}
			for _, key := range []string{"risk", "intent", "plan", "base", "head", "diff"} {
				value := expected[key]
				if value != nil && !sameValue(receipt[key], value) {
					errors = append(errors, "merge-review "+key+" does not match the current pull request")
				}
			}
			if receiptEntry != nil && !recordAuthorized(receiptEntry, mergeReviewRecord, "", contract.RiskAuthority) {
				errors = append(errors, "merge review was not posted by an authorized maintainer")
			}
			review := asMap(policy["review"])
			shared := any(nil)
			if review != nil {
				shared = review["shared_identity"]
			}
			if b, ok := asBool(shared); ok && !b {
				if receiptEntry != nil && asString(receiptEntry["surface"]) != "review" {
					errors = append(errors, "independent review receipt must be submitted as a native GitHub Review")
				}
				if receiptEntry != nil && sameValue(receiptEntry["author"], pullRequest["author"]) {
					errors = append(errors, "independent review requires an identity distinct from the pull request author")
				}
			}
		}

		quality := asMap(state["quality"])
		if quality == nil {
			quality = map[string]any{}
		}
		expectedQualityName := nested(policy, "quality", "check")
		qualityLabel := "None"
		if expectedQualityName != nil {
			if s, ok := expectedQualityName.(string); ok {
				qualityLabel = s
			} else {
				qualityLabel = asString(expectedQualityName)
				if qualityLabel == "" {
					qualityLabel = "None"
				}
			}
		}
		if receipt != nil && (asString(quality["name"]) != asString(expectedQualityName) || asString(quality["conclusion"]) != "success") {
			errors = append(errors, qualityLabel+" has not succeeded for the current head")
		}
	}

	if errors == nil {
		return []string{}
	}
	return errors
}
