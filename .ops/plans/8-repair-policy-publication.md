# Repair repository policy publication

Issue: #8
Current v1 intent: `sha256:a74c2bdfdeab6754e936558d086f72d097b11f8f85b2866d32e129e6acd4ea74`
Supersedes: `sha256:2c7509c62d420f84e3c18abcd93b4a9b9422404c7cd24a20c8e189937098d474`
Frozen merged base: `08f456934179cf641d5f66220a2cf397f62ba2a6` (`origin/main` at plan authoring)

## Approach

Repair the merged repository-policy publication path and simplify the unreleased contract before Issue #5 proceeds. The first pull request against this base (`#6`) left an unexecutable composite action, a stale quality-check mapping, duplicate policy check names, and strictness on generic Markdown headings and self-reported review counters.

Observed composite-action load failure (quote this scalar, not another):

```yaml
# actions/repository-policy/action.yml:8
    description: Stable evaluator surface: contract or merge-approval
```

PyYAML `safe_load` and GitHub's action metadata parser both reject that unquoted value: a `: ` inside an unquoted scalar is a mapping indicator (`ScannerError: mapping values are not allowed here` at column 42). Quote it:

```yaml
    description: "Stable evaluator surface: contract or merge-approval"
```

Do not describe `cache-dependency-glob: ${{ github.action_path }}/uv.lock` as the observed parse failure. PyYAML loads that glob as a string. Quote the glob only if a later independent GitHub/`with`-type check requires a quoted string; it is not this hotfix's root cause.

Also repair, in the same implementation once this plan is approved:

1. `quality.check` and `contracts/v0.1.0/repository-policy.schema.json` pin `CI / quality`, but `.github/workflows/ci.yml` publishes machine context `quality`. `build_live_state` selects `check_runs` whose `name` equals `policy.quality.check`.
2. Live `.github/workflows/policy.yml` defines Actions jobs named `contract` and `merge approval` and also POSTs Checks API results with those names, so PR Checks duplicate. `.github/workflows/repository-policy.yml` is the external `workflow_call` entry and currently repeats those job names. Keep that file; align it to one `publish` job and the same custom contexts. Central `policy.yml` must not call it.
3. Generic Issue/PR heading names and order are blocking findings. They become advisory: templates stay, the validator stops emitting blocking heading diagnostics.
4. `repo-ops.merge-review.v1` still binds `verdict`, `risk`, `intent`, `plan`, `base`, `head`, `diff`, `runtime`, and `model`. Remove `plan-round` and `implementation-round` from schema, `contract.json` field order, fixtures, validator, and tests.
5. Issue #5 Go compatibility (recorded here, applied to Issue #5 / PR #7 after this hotfix merges) preserves semantic findings, finding order, exit codes, canonical digests, check conclusions, and trust boundaries. It does not freeze Python-library diagnostic wording or whitespace.

Restore executable base-owned policy validation and publish one stable exact-head `contract` result and one stable exact-head `merge approval` result through a single internal publisher job. Preserve fail-closed revocation, immutable-base checkout, and machine contexts `quality`, `contract`, and `merge approval`. Rendered CI label remains `CI / quality` (workflow `CI` / job `quality`). The publisher job name must not be `contract`, `merge approval`, or `quality`.

This plan does not start Go, rewrite PR #6 or `main` history, change rulesets, enable enforcement, tag, release, or adopt consumers. It does not weaken exact-head, permission, plan-digest, changelog, or immutable-base controls.

PR #9 is the Issue #8 vehicle: this commit is plan-only; after owner `repo-ops.plan-approval.v1` naming this plan digest and plan commit, **the same pull request** receives implementation commits. Do not open a second implementation PR. This plan-only revision declares `Changelog: not-required`. The first implementation commit on this branch adds `changelog.d/8-repair-policy-publication.md` and the pull request body changelog declaration becomes `required`.

### Bootstrap exception (unattainable pre-merge live proof)

`policy.yml` is `pull_request_target` (plus review/comment) and checks out the **immutable base SHA**, not the pull-request head. Therefore PR #9 cannot execute its candidate `publish` job, candidate `action.yml`, or candidate Checks API sequence against itself. GitHub will keep running the broken `main` publisher until PR #9 squash-merges.

PR #9 merge gates are parser, unit, static alignment, and security review only. They do **not** include live publisher smoke, live composite load on this head, or injected publication-API faults.

Actual publisher/context smoke happens **after merge**, on rebased PR #7, and **gates starting Go**. Do not treat duplicated or red `contract` / `merge approval` results on PR #9 as implementation defects of this candidate, and do not invent a second workflow that runs head-owned policy just to obtain pre-merge evidence.

Rejected alternatives:

- Treating the unquoted cache glob as the action.yml parse failure.
- Keeping two Actions jobs and hiding duplicates in the UI.
- Renaming machine contexts to `CI / quality` or `Repository policy / contract`.
- Calling `.github/workflows/repository-policy.yml` from `policy.yml`.
- Deleting the reusable workflow instead of aligning it.
- Skipping revoke when evaluation will run in the same job.
- Compensating POST complexity that tries to force all-red after a successful contract publish.
- Claiming PR #9 can exercise the candidate `pull_request_target` path before merge.
- Deleting Issue/PR templates because headings become advisory.
- Keeping self-reported round counters “for the Go port”.
- Freezing `jsonschema` / PyYAML diagnostic strings or stdout whitespace as Issue #5 cutover gates.
- Starting Go, mutating PR #7, or rewriting Issue #5 before this hotfix merges.

## One-publisher-job control flow

Live central path is `.github/workflows/policy.yml`. External reusable path is `.github/workflows/repository-policy.yml` (`on: workflow_call`). Align both to one `publish` job and the same custom check names. `policy.yml` must not contain `uses: ./.github/workflows/repository-policy.yml`.

Replace `policy.yml` jobs `contract` and `merge-approval` with one job:

| Field | Value |
| --- | --- |
| Job id | `publish` |
| Job `name` | `publish` |
| Actions check label | `Repository policy / publish` (internal; not a required machine context) |
| Permissions | `contents: read`, `issues: read`, `pull-requests: read`, `checks: write` |
| Runner | `ubuntu-latest` |
| Job outputs | none required (single job; SHA stays in step outputs) |

Keep `intent-revocation` as a separate job with its current `if:` (Issue comments, not PR comments). It POSTs failure to `contract` and `merge approval` on linked open PR heads and must not be renamed to either context.

### Exact `publish` event predicate (`policy.yml`)

Copy this `if:` onto `publish` (today it is the `contract` job predicate). Do not invent a second policy job or `needs:`.

```yaml
if: >-
  github.event_name == 'pull_request_target' ||
  (
    github.event_name == 'issue_comment' &&
    github.event.issue.pull_request &&
    (
      github.event.comment.author_association == 'OWNER' ||
      github.event.comment.author_association == 'MEMBER' ||
      github.event.comment.author_association == 'COLLABORATOR'
    ) &&
    (
      contains(github.event.comment.body, 'repo-ops.plan-approval.v1') ||
      contains(github.event.comment.body, 'repo-ops.merge-review.v1') ||
      contains(github.event.changes.body.from, 'repo-ops.plan-approval.v1') ||
      contains(github.event.changes.body.from, 'repo-ops.merge-review.v1')
    )
  ) ||
  (
    github.event_name == 'pull_request_review' &&
    (
      github.event.review.author_association == 'OWNER' ||
      github.event.review.author_association == 'MEMBER' ||
      github.event.review.author_association == 'COLLABORATOR'
    ) &&
    (
      contains(github.event.review.body, 'repo-ops.plan-approval.v1') ||
      contains(github.event.review.body, 'repo-ops.merge-review.v1') ||
      contains(github.event.changes.body.from, 'repo-ops.plan-approval.v1') ||
      contains(github.event.changes.body.from, 'repo-ops.merge-review.v1')
    )
  )
```

### Exact SHA inputs and outputs (`policy.yml` resolve step)

Env in:

| Env | Expression |
| --- | --- |
| `EVENT_NAME` | `${{ github.event_name }}` |
| `DEFAULT_BASE_SHA` | `${{ github.event.pull_request.base.sha \|\| github.sha }}` |
| `DEFAULT_HEAD_SHA` | `${{ github.event.pull_request.head.sha \|\| github.sha }}` |
| `GH_TOKEN` | `${{ github.token }}` |

Logic:

- If `EVENT_NAME == issue_comment` and `jq -e '.issue.pull_request != null' "$GITHUB_EVENT_PATH"`: `gh api "repos/${GITHUB_REPOSITORY}/pulls/$(jq -r .issue.number "$GITHUB_EVENT_PATH")"` then `jq -r .base.sha` / `.head.sha`.
- Else write `DEFAULT_BASE_SHA` and `DEFAULT_HEAD_SHA`.
- Require both values match `^[0-9a-f]{40}$` or `exit 1`.
- Step outputs: `base-sha=<40-hex>`, `head-sha=<40-hex>` on `$GITHUB_OUTPUT`.
- Later steps read `steps.resolve.outputs.base-sha` as `CONTRACT_REF` and `steps.resolve.outputs.head-sha` as `HEAD_SHA`.

### Exact live permission predicate (revoke, and `intent-revocation`)

Env in for `publish` revoke:

| Env | Expression |
| --- | --- |
| `GH_TOKEN` | `${{ github.token }}` |
| `HEAD_SHA` | `${{ steps.resolve.outputs.head-sha }}` |
| `EVENT_NAME` | `${{ github.event_name }}` |
| `AUTHOR_ASSOCIATION` | `${{ github.event.comment.author_association \|\| github.event.review.author_association }}` |
| `AUTHOR_LOGIN` | `${{ github.event.comment.user.login \|\| github.event.review.user.login }}` |

Predicate (do not evaluate or publish if this step `exit 0`s without POSTs on a non-owner comment/review that is not `maintain`/`admin`; `pull_request_target` always revokes):

```bash
if [[ "$EVENT_NAME" != "pull_request_target" && "$AUTHOR_ASSOCIATION" != "OWNER" ]]; then
  permission="$(gh api "repos/${GITHUB_REPOSITORY}/collaborators/${AUTHOR_LOGIN}/permission" --jq .permission 2>/dev/null || true)"
  case "$permission" in
    maintain|admin) ;;
    *) exit 0 ;;
  esac
fi
```

Then POST `conclusion=failure` for `name=contract` and `name=merge approval` on `head_sha=${HEAD_SHA}` with title `Validation pending` and summary `Previous policy result revoked before candidate validation.` Both POSTs must succeed before evaluation. If either POST fails, fail the job without evaluating.

`intent-revocation` keeps the same collaborator lookup (`AUTHOR_ASSOCIATION != OWNER` → `maintain|admin` or `exit 0`) and scans `GET /repos/${GITHUB_REPOSITORY}/pulls?state=open&per_page=100` for bodies matching `(?im)^(Fixes|Closes|Related)[[:space:]]+#${ISSUE_NUMBER}[[:space:]]*$`.

### Exact step order (`policy.yml` `publish`)

1. Resolve `base-sha` / `head-sha` as above.
2. Revoke both custom checks with the permission predicate.
3. `[[ "$CONTRACT_REF" =~ ^[0-9a-f]{40}$ ]]` on `steps.resolve.outputs.base-sha`.
4. `git clone --filter=blob:none --no-checkout https://github.com/haesol-shin/.github.git .repo-ops` then `git -C .repo-ops checkout --detach "$CONTRACT_REF"`.
5. Evaluate contract: `uses: ./.repo-ops/actions/repository-policy` with `github-token: ${{ github.token }}` and `check: contract`. `id: contract` / `continue-on-error: true`.
6. Evaluate merge-approval: same action, `check: merge-approval`. `id: merge` / `continue-on-error: true`. The evaluator already inserts `contract check did not succeed; merge approval is blocked` when contract findings exist; still attempt a merge-approval publish.
7. **Sequential final POSTs** in one `if: always()` step after both evaluations, gated only by “resolve produced a 40-hex `HEAD_SHA`”:
   1. POST `name=contract`, `head_sha=${HEAD_SHA}`, `status=completed`, `conclusion=success` iff `steps.contract.outcome == success`, else `failure`. If this POST fails, **stop** (do not POST merge approval). Job fails. Contexts remain revoked red.
   2. POST `name=merge approval`, `head_sha=${HEAD_SHA}`, `status=completed`, `conclusion=success` iff **both** `steps.contract.outcome == success` and `steps.merge.outcome == success`, else `failure`. If this POST fails, **stop**. Do not re-POST `contract`.
8. Fail the job unless both evaluator outcomes were `success` **and** both sequential POSTs succeeded.

Do not create Actions jobs named `contract` or `merge approval`. Do not POST a third publisher of those names. Revoke POSTs are not final results; the sequential step-7 POSTs are the single final result per context that actually completed.

### Exact failure semantics (no compensating all-red)

Final POSTs are ordered. A successful contract POST is not rolled back. Do not add a best-effort second failure POST to “fix” a later merge-approval POST fault.

| Fault | Latest `contract` | Latest `merge approval` | Job |
| --- | --- | --- | --- |
| Revoke POST fails | no success published | no success published | failure |
| Checkout / contract-ref / clone fails after successful revoke | revoked red | revoked red | failure |
| Contract evaluator non-success; both POSTs succeed | published `failure` | published `failure` | failure |
| Contract evaluator success, merge-approval evaluator non-success; both POSTs succeed | published `success` | published `failure` | failure |
| Contract final POST fails | revoked red | revoked red | failure |
| Contract success already published; merge-approval POST fails | **may remain green** | revoked red | failure |
| Job cancelled after revoke and before any final POST | revoked red | revoked red | failure |

Guarantees:

- Any fault fails the `publish` job.
- Any fault leaves **at least one** required policy context red (`contract` or `merge approval`).
- Do **not** claim false all-red when contract success already published.

Never use `continue-on-error` on revoke or on the sequential publish step.

### External reusable workflow

Keep `.github/workflows/repository-policy.yml` as the consumer-facing `workflow_call`.

Inputs (unchanged names):

| Input | Type | Meaning |
| --- | --- | --- |
| `contract-ref` | string, required | Immutable 40-hex SHA of trusted code |
| `head-sha` | string, required | Exact head receiving custom checks |

Replace jobs `revoke`, `contract`, and `merge-approval` with one job `publish` / `name: publish` that uses `inputs['contract-ref']` and `inputs['head-sha']` directly (no event resolve). Same revoke → evaluate → sequential POST control flow and the same custom check names. Caller permission remains the caller's `checks: write` plus read scopes. Do not name reusable jobs `contract` or `merge approval`.

### Quality selection

Machine context is `quality`, not `CI / quality`.

| Surface | Required value |
| --- | --- |
| `.github/repo-policy.yml` `quality.check` | `quality` |
| `contracts/v0.1.0/repository-policy.schema.json` `properties.quality.properties.check.const` | `quality` |
| Fixture `policy.quality.check` and live-state `quality.name` in `fixtures/valid-high.json` and `fixtures/valid-low-direct.json` | `quality` |
| `.github/workflows/ci.yml` `jobs.quality.name` | `quality` (already correct; do not rename) |
| Collector | unchanged algorithm: last `check_runs` entry whose `name == policy.quality.check` |

Extending fixtures inherit the `valid-high.json` policy object; do not re-pin `CI / quality` there. Do not change `quality.commands`. Do not treat the UI label `CI / quality` as a check-run `name`.

### Composite action load

Quote `actions/repository-policy/action.yml` line 8. Keep `inputs.check` (`contract` | `merge-approval`, default `merge-approval`). Do not reintroduce `inputs.mode`. Nested `enable-cache: true` may remain a boolean. Do not switch off `astral-sh/setup-uv@37802adc94f370d6bfd71619e3f0bf239e1f3b78` in this change.

### Manifest type validation and static pins

Add CI coverage inside the existing `python -m unittest discover -s tests -v` job (no new workflow). Assert, at minimum:

- `yaml.safe_load` of `actions/repository-policy/action.yml` succeeds (this fails on `main` today because of line 8).
- `runs.using == "composite"`.
- `inputs.check` exists and its `description` is a Python `str`.
- The file text contains `description: "Stable evaluator surface: contract or merge-approval"`.
- Every composite `run` step declares `shell`.

Do not add a glob-quote assertion unless an independent type check proves GitHub rejects the current glob.

Replace `test_contract_templates_and_schemas_stay_aligned` pins that require Actions job names `contract` and `merge approval` or `needs: contract`. New pins:

- Both `policy.yml` and `repository-policy.yml` contain `name: publish`, `-f "name=contract"`, `-f "name=merge approval"`.
- `policy.yml` contains `uses: ./.repo-ops/actions/repository-policy` and `-f "head_sha=${HEAD_SHA}"`.
- `policy.yml` does not contain `name: contract`, `name: merge approval`, `needs: contract`, `needs: prepare`, `needs: revoke`, or `uses: ./.github/workflows/repository-policy.yml`.
- `repository-policy.yml` does not contain `name: contract`, `name: merge approval`, or `needs: [revoke, contract]`.
- `repository-policy.yml` still has `on: workflow_call` and inputs `contract-ref` / `head-sha`.
- `ci.yml` still contains `  quality:\n    name: quality`.
- Fail-closed **sequencing** (unit/static, pre-merge): revoke step text appears before both `uses: ./.repo-ops/actions/repository-policy` occurrences; the contract Checks API POST appears in file order before the merge-approval Checks API POST; the publish step is `if: always()`; revoke and publish steps do not set `continue-on-error`.

Templates remain aligned with the advisory heading lists. Alignment tests may still assert template files contain those headings; they must not require the validator to fail when a live Issue/PR omits or reorders them.

## Contract simplifications

### Advisory headings

Keep `.github/ISSUE_TEMPLATE/work_item.md` and `.github/PULL_REQUEST_TEMPLATE.md` with the current level-two names and order. Keep `contract.json` heading lists as documentation for those templates.

Change `validate_heading_contract` / `validate_state` so missing, duplicate, or reordered generic Issue/PR headings are **not** findings. Delete or rewrite tests that currently require:

- `pull request body is missing …`
- `linked issue body is missing …`
- `… contains duplicate ## Summary`
- `… required headings must appear in contract order`

Do not treat `## Completion criteria` on Issue #5 as a contract blocker after this change. Changelog declarations, authority lines, risk lines, and fenced machine records remain blocking.

### Merge-review records without round counters

`repo-ops.merge-review.v1` field order becomes:

```text
verdict risk intent plan base head diff runtime model
```

Updates:

- `contracts/v0.1.0/merge-review.schema.json`: drop `plan-round` and `implementation-round` from `required` and `properties`.
- `contracts/v0.1.0/contract.json` `records.repo-ops.merge-review.v1.field_order`: same list (no round fields).
- Remove `round_limits` from `contract.json` if nothing else reads it after validator edits.
- `validate.py`: stop `parse_round` enforcement on merge receipts; keep binding `risk`, `intent`, `plan`, `base`, `head`, `diff` (and existing `runtime` / `model` schema checks).
- Fixtures (`valid-high.json`, `valid-low-direct.json`, `valid-v1-related.json`, `invalid-*.json` merge-review bodies): delete the two tokens.
- Tests that mention `plan-round:none` or `n/5` round limits: delete or retarget to remaining machine fields.

Preserve plan-approval records (`decision risk issue intent plan plan-commit`) unchanged.

### Issue #5 Go comparison bar (post-merge documentation)

Issue #8 implementation does not start Go. After this hotfix is on `main`, update Issue #5 intent/body and rebase/update PR #7 so the Go migration requires:

- Same semantic findings (codes/meaning), finding order, exit codes, canonical intent/plan/diff digests, check conclusions, and trust boundaries (immutable base, no PR-head execution, token non-leakage, specified `git diff` byte stream for `diff_digest`).
- Not required: byte-identical `jsonschema==4.25.1` diagnostic strings, PyYAML error wording, or stdout/stderr whitespace beyond the semantic `::warning title=Repository policy::{finding}` payload order.

## PR #9 lifecycle

Do not mutate Issue #5, PR #7, or Go sources until this hotfix is on `main`.

1. Land this plan-only revision on PR #9. Diff remains only `.ops/plans/8-repair-policy-publication.md`. This draft is not approval, merge, release, or ruleset authority.
2. Owner posts `repo-ops.plan-approval.v1` on PR #9 naming this file's plan digest and this plan commit.
3. **Same PR #9** receives implementation commits (do not open a second Issue #8 PR). First implementation commit adds `changelog.d/8-repair-policy-publication.md` and the PR body switches to `Changelog: required`.
4. Merge PR #9 only after the **PR #9 merge acceptance** section below. Squash-merge using repository defaults `PR_TITLE` + `BLANK`. Do not change rulesets. Do not tag or release.
5. Perform **PR #7 recovery acceptance** after merge. Do not start Go until that section passes.

## Execution

After plan approval, implementation commits on this branch touch:

- `actions/repository-policy/action.yml` (quote line 8)
- `.github/workflows/policy.yml` (one `publish` job; inlined predicate/SHA/permission as specified)
- `.github/workflows/repository-policy.yml` (keep `workflow_call`; one `publish` job; same custom contexts)
- `.github/repo-policy.yml` (`quality.check: quality`)
- `contracts/v0.1.0/repository-policy.schema.json` (`const: "quality"`)
- `contracts/v0.1.0/merge-review.schema.json` and `contracts/v0.1.0/contract.json` (drop round fields; headings advisory)
- `actions/repository-policy/validate.py` (no blocking generic heading findings; no round-counter enforcement)
- fixtures that pin `CI / quality`, heading failures, or merge-review round tokens
- `tests/test_validator.py` (alignment, manifest load, sequencing pins, deleted heading/round pins)
- `changelog.d/8-repair-policy-publication.md`

Do not edit `validate.py` quality collector algorithm unless a test proves it still compares against `CI / quality` after the policy/schema change. Do not edit `ci.yml` job name. Do not add ruleset JSON. Do not quote the cache glob unless an independent type failure is demonstrated.

## Commands

Working directory is the repository root. Plan-only revisions do not run the implementation matrix.

### PR #9 merge-gate commands (candidate tree, not live `pull_request_target`)

Reproduce today's action.yml failure on `main`; must raise `ScannerError`:

```text
python -c "import yaml; from pathlib import Path; yaml.safe_load(Path('actions/repository-policy/action.yml').read_text(encoding='utf-8'))"
```

After the quote, the same command returns a mapping and:

```text
python -c "from pathlib import Path; text = Path('actions/repository-policy/action.yml').read_text(encoding='utf-8'); assert 'description: \"Stable evaluator surface: contract or merge-approval\"' in text"
```

Conformance, including new manifest and sequencing assertions:

```text
uv run --project actions/repository-policy --locked python -m unittest discover -s tests -v
```

Patch hygiene:

```text
git diff --check origin/main...HEAD
```

Static no-call pin:

```text
python -c "from pathlib import Path; t=Path('.github/workflows/policy.yml').read_text(encoding='utf-8'); assert 'uses: ./.github/workflows/repository-policy.yml' not in t"
```

### Post-merge PR #7 smoke commands (gates starting Go)

After hotfix squash-merge:

```text
git fetch origin
git checkout plan/5-go-validator
git rebase origin/main
# edit .ops/plans/5-migrate-repository-policy-validator-to-go.md for the semantic comparison bar
git push --force-with-lease
```

On the rebased PR #7 exact head `$HEAD`:

```text
gh api --paginate "repos/haesol-shin/.github/commits/${HEAD}/check-runs?per_page=100" \
  --jq '.check_runs | group_by(.name) | map({name: .[0].name, n: length, latest: (sort_by(.id) | last | {conclusion, started_at, id})})'
```

Accept:

- Latest `quality` is `success` when CI passed.
- Latest `contract` and `merge approval` are custom results; no Actions job named `contract` or `merge approval`.
- An Actions job named `publish` may appear.
- No check-run name `CI / quality`.
- The `publish` job log on that head gets past `uses: ./.repo-ops/actions/repository-policy` without a YAML/`action.yml` parse error.

Revoke-then-final may yield `n >= 2` API rows per policy context. Forbidden is a parallel GitHub Actions job whose `name` equals `contract` or `merge approval`.

## Verification and recovery

### Plan-only revision (this change)

Diff must contain only `.ops/plans/8-repair-policy-publication.md`. No workflow, action, schema, fixture, test, changelog, Issue, or PR #7 mutation.

Expected live checks on this exact head against unrepaired `main` are **not merge gates**: `quality` should pass if unittest discovery ignores `.ops/plans/`; `contract` / `merge approval` may be missing, duplicated, or fail closed. This revision must not claim those contexts are green.

### PR #9 merge acceptance (implementation commits on this PR)

Merge is allowed only when all of the following hold. None of these require the candidate `publish` job to have run on PR #9.

- `yaml.safe_load(action.yml)` succeeds; quoted line 8 is present; manifest unit test fails if those quotes are removed.
- `quality.check` schema const, repo policy, and fixtures equal `quality`.
- Validator emits no blocking findings for missing/reordered generic headings; templates still contain them.
- Merge-review fixtures and schema have no `plan-round` / `implementation-round`; receipts still bind risk, intent, plan, base, head, diff, runtime, model.
- `python -m unittest discover -s tests -v` passes, including sequencing and alignment pins.
- `git diff --check origin/main...HEAD` is silent.
- `policy.yml` does not call `repository-policy.yml`; both files have exactly one Actions job named `publish` for policy evaluation/publication; neither defines Actions jobs named `contract` or `merge approval`.
- Security review of the implementation head: immutable-base checkout, `pull_request_target` does not execute PR-head code, token is not printed, revoke precedes evaluate, final POSTs are sequential, no compensating all-red logic.
- Owner plan approval names this plan digest and the plan commit that introduced it (or a later reviewed plan commit on this branch).

Explicitly **not** a PR #9 merge gate: live unique Checks API contexts on the PR #9 head, live composite load via `pull_request_target`, or injected Checks API publication failures.

### PR #7 recovery acceptance (post-merge only; gates starting Go)

After squash-merge (`PR_TITLE` + `BLANK`):

- Issue #5 body uses the standard template (including `## Acceptance`) and a superseding intent records the semantic Go comparison bar.
- PR #7 rebases onto new `origin/main`; its plan no longer freezes Python-library diagnostics/whitespace or merge-review round fields, and it names machine contexts `quality`, `contract`, and `merge approval`.
- On the rebased PR #7 head, the smoke command shows unique latest `quality`, `contract`, and `merge approval`; publisher job is `publish`; composite action loaded.
- Exact runtime fail-closed behavior is observed there: a failed evaluator leaves the corresponding final custom check red; a `publish` job failure leaves at least one required policy context red. Do not require a synthetic publication-API fault injection on PR #9.

If that smoke fails, do not start Go; repair on a follow-up against `main` or revert the squash commit. Do not change rulesets to compensate.

### Negative cases

- Unquoted `description: Stable evaluator surface: contract or merge-approval` must fail `yaml.safe_load` and must not ship.
- Asserting the cache glob as the load failure is a plan defect, not an implementation task.
- `quality.check: CI / quality` must fail schema validation.
- Reintroducing Actions job names `contract` or `merge approval` in `policy.yml` or `repository-policy.yml` must fail alignment tests.
- Wiring `policy.yml` to `repository-policy.yml` remains forbidden.
- Reintroducing merge-review round fields must fail schema/field-order tests.
- Restoring blocking generic heading findings must fail the new advisory tests.
- Claiming PR #9 live `contract`/`merge approval` uniqueness as a merge gate must be rejected (bootstrap exception).

### Explicit non-authority

This plan does not authorize: implementing before owner plan approval; changing rulesets; enabling required checks; tagging or releasing `v0.1.0`; closing Issue #3, #5, or #8 from the plan-only revision; starting Go; rewriting `main` or PR #6; keeping duplicate same-name Actions jobs; mutating Issue #5 or PR #7 before the hotfix merge; treating unrepaired-base check duplication on PR #9 as a candidate-workflow result.

If implementation review on PR #9 fails, repair that same branch. If post-merge PR #7 smoke still shows duplicate job names or `action.yml` still fails to load, do not start Go. Rollback of a merged hotfix is revert of that squash commit.
