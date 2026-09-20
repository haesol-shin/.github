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
2. Live `.github/workflows/policy.yml` defines Actions jobs named `contract` and `merge approval` and also POSTs Checks API results with those names, so PR Checks duplicate. Unused `.github/workflows/repository-policy.yml` repeats the job names.
3. Generic Issue/PR heading names and order are blocking findings. They become advisory: templates stay, the validator stops emitting blocking heading diagnostics.
4. `repo-ops.merge-review.v1` still binds `verdict`, `risk`, `intent`, `plan`, `base`, `head`, `diff`, `runtime`, and `model`. Remove `plan-round` and `implementation-round` from schema, `contract.json` field order, fixtures, validator, and tests.
5. Issue #5 Go compatibility (recorded here, applied to Issue #5 / PR #7 after this hotfix merges) preserves semantic findings, finding order, exit codes, canonical digests, check conclusions, and trust boundaries. It does not freeze Python-library diagnostic wording or whitespace.

Restore executable base-owned policy validation and publish one stable exact-head `contract` result and one stable exact-head `merge approval` result through a single internal publisher job. Preserve fail-closed revocation, immutable-base checkout, and machine contexts `quality`, `contract`, and `merge approval`. Rendered CI label remains `CI / quality` (workflow `CI` / job `quality`). The publisher job name must not be `contract`, `merge approval`, or `quality`.

This plan does not start Go, rewrite PR #6 or `main` history, change rulesets, enable enforcement, tag, release, or adopt consumers. It does not weaken exact-head, permission, plan-digest, changelog, or immutable-base controls.

PR #9 is the Issue #8 vehicle: this commit is plan-only; after owner `repo-ops.plan-approval.v1` naming this plan digest and plan commit, **the same pull request** receives implementation commits. Do not open a second implementation PR. This plan-only revision declares `Changelog: not-required`. The first implementation commit on this branch adds `changelog.d/8-repair-policy-publication.md` and the pull request body changelog declaration becomes `required`.

Rejected alternatives:

- Treating the unquoted cache glob as the action.yml parse failure.
- Keeping two Actions jobs and hiding duplicates in the UI.
- Renaming machine contexts to `CI / quality` or `Repository policy / contract`.
- Calling `.github/workflows/repository-policy.yml` from `policy.yml`.
- Skipping revoke when evaluation will run in the same job.
- Publishing success when the evaluator or the Checks API POST fails.
- Deleting Issue/PR templates because headings become advisory.
- Keeping self-reported round counters “for the Go port”.
- Freezing `jsonschema` / PyYAML diagnostic strings or stdout whitespace as Issue #5 cutover gates.
- Starting Go, mutating PR #7, or rewriting Issue #5 before this hotfix merges.

## One-publisher-job control flow

Live path remains `.github/workflows/policy.yml` (`pull_request_target`, `pull_request_review`, `issue_comment`). Do not add `uses: ./.github/workflows/repository-policy.yml`.

Replace jobs `contract` and `merge-approval` with one job:

| Field | Value |
| --- | --- |
| Job id | `publish` |
| Job `name` | `publish` |
| Actions check label | `Repository policy / publish` (internal; not a required machine context) |
| Permissions | `contents: read`, `issues: read`, `pull-requests: read`, `checks: write` |
| Runner | `ubuntu-latest` |

Keep `intent-revocation` as a separate job. It already POSTs failure to `contract` and `merge approval` and must not be renamed to either context.

`publish` `if:` is the current union used by the `contract` job (pull_request_target, or trusted plan/merge-review comments/reviews). Do not depend on a second policy job.

Exact step order:

1. **Resolve** immutable base SHA and exact head SHA (reuse the current `contract` resolve script, including `issue_comment` → `gh api repos/.../pulls/{n}`). Fail the job if either value is not `[0-9a-f]{40}`.
2. **Revoke both exact-head custom checks before evaluation.** POST Checks API `conclusion=failure` for `name=contract` and `name=merge approval` on `head_sha=<exact head>` with title `Validation pending` and summary `Previous policy result revoked before candidate validation.` Keep the current non-owner collaborator permission gate used by today's revoke step. Do not evaluate before both POSTs succeed. If either revoke POST fails, fail the job without evaluating.
3. **Validate** the base SHA is a full 40-hex contract-ref.
4. **Fetch** trusted code: `git clone --filter=blob:none --no-checkout https://github.com/haesol-shin/.github.git .repo-ops` then `git -C .repo-ops checkout --detach "$CONTRACT_REF"`. Never execute pull-request code.
5. **Evaluate contract** with `uses: ./.repo-ops/actions/repository-policy` and `check: contract`. `continue-on-error: true`. Record `steps.<id>.outcome`.
6. **Evaluate merge-approval** with the same composite action and `check: merge-approval`. `continue-on-error: true`. Record outcome. The evaluator already inserts `contract check did not succeed; merge approval is blocked` when contract findings exist; still publish a merge-approval check.
7. **Publish exactly one final custom check per context** in an `if: always()` step after both evaluations:
   - POST `name=contract`, `head_sha=<exact head>`, `status=completed`, `conclusion=success` iff the contract step outcome is `success`, else `failure`.
   - POST `name=merge approval`, `head_sha=<exact head>`, `status=completed`, `conclusion=success` iff **both** evaluator outcomes are `success`, else `failure`.
8. **Fail the job** unless both final conclusions are `success` **and** both POSTs succeeded.

Do not create Actions jobs named `contract` or `merge approval`. Do not POST a third publisher of those names. Revoke POSTs are not final results; the step-7 POSTs are the single final result per context.

### Exact failure semantics

| Fault | `contract` final | `merge approval` final | Job |
| --- | --- | --- | --- |
| Revoke POST fails | no success published | no success published | failure |
| Checkout / contract-ref / clone fails after successful revoke | remains revoked red | remains revoked red | failure |
| Contract evaluator non-success (findings or crash) | published `failure` | published `failure` | failure |
| Contract evaluator success, merge-approval evaluator non-success | published `success` | published `failure` | failure |
| Evaluator success, Checks API POST fails | do not publish `success`; prior revoke leaves red | same | failure |
| Job cancelled or runner dies after revoke and before final POST | remains revoked red | remains revoked red | failure |

Never use `continue-on-error` on revoke or on the final publish step. Never skip the final POSTs when an evaluator failed (`if: always()` after evaluations, gated only by “revoke and resolve already produced an exact head SHA”). Evaluator or publication faults must leave both required policy contexts red, except the one case where contract evaluation succeeded and only merge-approval failed: `contract` may be green and `merge approval` must be red.

`intent-revocation` keeps posting `conclusion=failure` for both custom names on linked open PR heads. That is revocation, not a second publisher job.

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

### Manifest type validation

Add CI coverage inside the existing `python -m unittest discover -s tests -v` job (no new workflow). Assert, at minimum:

- `yaml.safe_load` of `actions/repository-policy/action.yml` succeeds (this fails on `main` today because of line 8).
- `runs.using == "composite"`.
- `inputs.check` exists and its `description` is a Python `str`.
- The file text contains `description: "Stable evaluator surface: contract or merge-approval"` (text assertion so a future unquoted colon cannot hide behind a different loader).
- Every composite `run` step declares `shell`.

Do not add a glob-quote assertion unless an independent type check proves GitHub rejects the current glob.

Replace `test_contract_templates_and_schemas_stay_aligned` pins that require Actions job names `contract` and `merge approval` or `needs: contract`. New pins:

- `policy.yml` contains `name: publish`, `uses: ./.repo-ops/actions/repository-policy`, `-f "name=contract"`, `-f "name=merge approval"`, `-f "head_sha=${HEAD_SHA}"`.
- `policy.yml` does not contain `name: contract`, `name: merge approval`, `needs: contract`, `needs: prepare`, `needs: revoke`, or `uses: ./.github/workflows/repository-policy.yml`.
- `ci.yml` still contains `  quality:\n    name: quality`.
- `repository-policy.yml` must not define Actions jobs named `contract` or `merge approval`. Either restyle it to the same `publish` job (still unused by `policy.yml`) or delete the duplicate job names.

Templates remain aligned with the advisory heading lists. Alignment tests may still assert template files contain those headings; they must not require the validator to fail when a live Issue/PR omits or reorders them.

### Unused reusable workflow

`.github/workflows/repository-policy.yml` is not the live `pull_request_target` path. Do not wire it. If it remains in-tree, give it the same one-publisher semantics or remove the two same-name jobs. Do not leave alignment tests requiring `needs: [revoke, contract]` on that file.

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

## PR #9 lifecycle and Issue #5 / PR #7 recovery

Do not mutate Issue #5, PR #7, or Go sources until this hotfix is on `main`.

1. Land this plan-only revision on PR #9. Diff remains only `.ops/plans/8-repair-policy-publication.md`. This draft is not approval, merge, release, or ruleset authority.
2. Owner posts `repo-ops.plan-approval.v1` on PR #9 naming this file's plan digest and this plan commit.
3. **Same PR #9** receives implementation commits (do not open a second Issue #8 PR). First implementation commit adds `changelog.d/8-repair-policy-publication.md` and the PR body switches to `Changelog: required`. Implementation round counters are not machine-validated; keep review tight anyway.
4. Squash-merge PR #9 using repository defaults `PR_TITLE` + `BLANK`. Do not change rulesets. Do not tag or release.
5. **After merge:** update Issue #5 body to the standard template (including `## Acceptance`) and post a superseding Issue #5 intent that records the semantic Go comparison bar above. Rebase PR #7 (`plan/5-go-validator`) onto new `origin/main` and edit its plan so it no longer freezes Python-library diagnostics/whitespace or merge-review round fields, and so it names the repaired machine contexts. Then verify Checks API uniqueness on the rebased head.
6. Do not start Go from Issue #8.

## Execution

After plan approval, implementation commits on this branch touch:

- `actions/repository-policy/action.yml` (quote line 8)
- `.github/workflows/policy.yml` (one `publish` job)
- `.github/workflows/repository-policy.yml` (remove same-name jobs or align)
- `.github/repo-policy.yml` (`quality.check: quality`)
- `contracts/v0.1.0/repository-policy.schema.json` (`const: "quality"`)
- `contracts/v0.1.0/merge-review.schema.json` and `contracts/v0.1.0/contract.json` (drop round fields; headings advisory)
- `actions/repository-policy/validate.py` (no blocking generic heading findings; no round-counter enforcement)
- fixtures that pin `CI / quality`, heading failures, or merge-review round tokens
- `tests/test_validator.py` (alignment, manifest load, deleted heading/round pins)
- `changelog.d/8-repair-policy-publication.md`

Do not edit `validate.py` quality collector algorithm unless a test proves it still compares against `CI / quality` after the policy/schema change. Do not edit `ci.yml` job name. Do not add ruleset JSON. Do not quote the cache glob unless an independent type failure is demonstrated.

## Commands

Working directory is the repository root. Plan-only revisions do not run the implementation matrix.

Reproduce today's action.yml failure:

```text
python -c "import yaml; from pathlib import Path; yaml.safe_load(Path('actions/repository-policy/action.yml').read_text(encoding='utf-8'))"
```

Must raise `ScannerError` on `main`. After the quote, the same command returns a mapping and:

```text
python -c "from pathlib import Path; text = Path('actions/repository-policy/action.yml').read_text(encoding='utf-8'); assert 'description: \"Stable evaluator surface: contract or merge-approval\"' in text"
```

Conformance, including new manifest assertions:

```text
uv run --project actions/repository-policy --locked python -m unittest discover -s tests -v
```

Patch hygiene:

```text
git diff --check origin/main...HEAD
```

Context uniqueness on an exact head (replace `$HEAD` with the 40-hex SHA). Latest completed run per name is the final result:

```text
gh api --paginate "repos/haesol-shin/.github/commits/${HEAD}/check-runs?per_page=100" \
  --jq '.check_runs | group_by(.name) | map({name: .[0].name, n: length, latest: (sort_by(.id) | last | {conclusion, started_at, id})})'
```

Accept on a completed `publish` workflow:

- Latest `quality` conclusion is `success` when CI passed.
- Latest `contract` is the single final custom result (not an Actions job named `contract`).
- Latest `merge approval` is the single final custom result (not an Actions job named `merge approval`).
- An Actions job named `publish` may appear as `publish` or `Repository policy / publish`.
- No check-run name `CI / quality`.
- No second live Actions job named `contract` or `merge approval`.

Revoke-then-final may yield `n >= 2` API rows per policy context (revoke failure, then final POST). That is required fail-closed behavior. Forbidden is a parallel GitHub Actions job whose `name` equals `contract` or `merge approval`.

Live composite load evidence: the `publish` job must get past `uses: ./.repo-ops/actions/repository-policy` without a YAML/`action.yml` parse error. A red `contract` check from remaining policy findings is acceptable; `ScannerError` or GitHub “Invalid action” on line 8 is not.

After hotfix squash-merge, rebase PR #7:

```text
git fetch origin
git checkout plan/5-go-validator
git rebase origin/main
# edit .ops/plans/5-migrate-repository-policy-validator-to-go.md for the semantic comparison bar
git push --force-with-lease
```

## Verification and recovery

### Plan-only revision (this change)

Diff must contain only `.ops/plans/8-repair-policy-publication.md`. No workflow, action, schema, fixture, test, changelog, Issue, or PR #7 mutation.

Expected live checks on this exact head against unrepaired `main`:

- `quality` — pass if unittest discovery ignores `.ops/plans/`.
- `contract` / `merge approval` — may be missing, duplicated, or fail closed because the merged publisher is broken (duplicate same-name jobs plus unquoted line 8). This revision must not claim those contexts are green.

Unverified on this head: unique final `contract` and `merge approval` publication. That evidence is an acceptance gate of the implementation commits on PR #9, then again on rebased PR #7.

### Implementation acceptance (later commits on PR #9)

- `yaml.safe_load(action.yml)` succeeds; quoted line 8 is present; composite action loads in a real `publish` job.
- Manifest unit test fails if line 8 quotes are removed.
- `quality.check` schema const, repo policy, and fixtures equal `quality`.
- Validator emits no blocking findings for missing/reordered generic headings; templates still contain them.
- Merge-review fixtures and schema have no `plan-round` / `implementation-round`; receipts still bind risk, intent, plan, base, head, diff, runtime, model.
- `python -m unittest discover -s tests -v` passes, including updated alignment pins.
- Checks API on the implementation head: latest `quality`, `contract`, and `merge approval` exist; no Actions job named `contract` or `merge approval`; publisher job is `publish`.
- Forced evaluator failure publishes red `contract` and red `merge approval`.
- Forced publication failure after revoke leaves both policy contexts red.
- After squash-merge (`PR_TITLE` + `BLANK`), Issue #5 body/intent and PR #7 plan are updated; Checks API uniqueness is recorded on the rebased PR #7 head.

### Negative cases

- Unquoted `description: Stable evaluator surface: contract or merge-approval` must fail `yaml.safe_load` and must not ship.
- Asserting the cache glob as the load failure is a plan defect, not an implementation task.
- `quality.check: CI / quality` must fail schema validation.
- Reintroducing Actions job names `contract` or `merge approval` in `policy.yml` must fail alignment tests.
- Wiring `policy.yml` to `repository-policy.yml` remains forbidden.
- Reintroducing merge-review round fields must fail schema/field-order tests.
- Restoring blocking generic heading findings must fail the new advisory tests.

### Explicit non-authority

This plan does not authorize: implementing before owner plan approval; changing rulesets; enabling required checks; tagging or releasing `v0.1.0`; closing Issue #3, #5, or #8 from the plan-only revision; starting Go; rewriting `main` or PR #6; keeping duplicate same-name Actions jobs; mutating Issue #5 or PR #7 before the hotfix merge.

If implementation review on PR #9 fails, repair that same branch. If live publication is still duplicated or `action.yml` still fails to load, do not rebase PR #7 and do not start Go. Rollback of a merged hotfix is revert of that squash commit; do not change rulesets to compensate.
