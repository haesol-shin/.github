# Repair repository policy publication

Issue: #8
Current v1 intent: `sha256:2c7509c62d420f84e3c18abcd93b4a9b9422404c7cd24a20c8e189937098d474`
Frozen merged base: `08f456934179cf641d5f66220a2cf397f62ba2a6` (`origin/main` at plan authoring)

## Approach

Repair the merged repository-policy publication path before Issue #5 proceeds. The first pull request against this base (`#6`) left three publication defects:

1. Composite action `actions/repository-policy/action.yml` line 18 is the invalid manifest scalar `cache-dependency-glob: ${{ github.action_path }}/uv.lock`. GitHub's action metadata schema requires that `with` value to be a string; the unquoted `${{ ... }}` form prevents the composite action from loading in a real base-owned `policy.yml` run.
2. `quality.check` and `contracts/v0.1.0/repository-policy.schema.json` pin `CI / quality`, but `.github/workflows/ci.yml` publishes machine context `quality` (`jobs.quality.name: quality` under workflow `CI`). The collector in `build_live_state` selects `check_runs` whose `name` equals `policy.quality.check`, so merge approval cannot see a successful quality receipt on the exact head.
3. Live `.github/workflows/policy.yml` defines GitHub Actions jobs whose `name` values are `contract` and `merge approval`, then POSTs Checks API results with those same names. GitHub Actions already publishes a check run for each job name, so PR Checks show duplicate same-name results. Unused `.github/workflows/repository-policy.yml` repeats the same two job names.

Restore executable base-owned policy validation and publish one stable exact-head `contract` result and one stable exact-head `merge approval` result through a single internal publisher job. Preserve fail-closed revocation, immutable-base checkout, and the approved machine contexts `quality`, `contract`, and `merge approval`. Rendered labels remain `CI / quality` (workflow `CI` / job `quality`). Custom Checks API names remain `contract` and `merge approval`; the publisher job name must not be either of those strings and must not be `quality`.

This plan does not start Go, rewrite PR #6 or `main` history, change rulesets, enable enforcement, tag, release, or adopt consumers. Issue #5 remains blocked until this hotfix merges and PR #7 is rebased onto the repaired base. Issue #8 implementation pull requests use `Related #8`. This plan-only pull request declares `Changelog: not-required`. The later implementation pull request declares `Changelog: required` and adds one `changelog.d/8-repair-policy-publication.md` fragment.

Rejected alternatives:

- Keeping two Actions jobs and hiding duplicates in the UI.
- Renaming machine contexts to `CI / quality` or `Repository policy / contract`.
- Calling `.github/workflows/repository-policy.yml` from `policy.yml`.
- Skipping revoke when evaluation will run in the same job.
- Publishing success when the evaluator or the Checks API POST fails.
- Starting Issue #5 Go work, mutating PR #7, or editing Issue #5 from this plan-only pull request.

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
2. **Revoke both exact-head custom checks before evaluation.** POST Checks API `conclusion=failure` for `name=contract` and `name=merge approval` on `head_sha=<exact head>` with title `Validation pending` and summary `Previous policy result revoked before candidate validation.` Keep the current non-owner collaborator permission gate used by today's revoke step. Do not evaluate before both POSTs succeed. If either revoke POST fails, fail the job without evaluating (no green policy result exists yet).
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

Quote the invalid scalar in `actions/repository-policy/action.yml`:

```yaml
cache-dependency-glob: "${{ github.action_path }}/uv.lock"
```

Keep `inputs.check` (`contract` | `merge-approval`, default `merge-approval`). Do not reintroduce `inputs.mode`. Nested `enable-cache: true` may remain a boolean. The quoted glob is the load fix; do not switch the composite action off of `uses: astral-sh/setup-uv@37802adc94f370d6bfd71619e3f0bf239e1f3b78` in this hotfix.

### Manifest type validation

Add CI coverage inside the existing `python -m unittest discover -s tests -v` job (no new workflow). Assert, at minimum:

- `yaml.safe_load` of `actions/repository-policy/action.yml` succeeds.
- `runs.using == "composite"`.
- `inputs.check` exists.
- The file text contains the quoted glob `cache-dependency-glob: "${{ github.action_path }}/uv.lock"` (text assertion; PyYAML may coerce the unquoted form to `str` and would not catch the GitHub load failure alone).
- After load, that `with` value is a Python `str`.
- Every composite `run` step declares `shell`.

Replace `test_contract_templates_and_schemas_stay_aligned` pins that require Actions job names `contract` and `merge approval` or `needs: contract`. New pins:

- `policy.yml` contains `name: publish`, `uses: ./.repo-ops/actions/repository-policy`, `-f "name=contract"`, `-f "name=merge approval"`, `-f "head_sha=${HEAD_SHA}"`.
- `policy.yml` does not contain `name: contract`, `name: merge approval`, `needs: contract`, `needs: prepare`, `needs: revoke`, or `uses: ./.github/workflows/repository-policy.yml`.
- `ci.yml` still contains `  quality:\n    name: quality`.
- `repository-policy.yml` must not define Actions jobs named `contract` or `merge approval`. Either restyle it to the same `publish` job (still unused by `policy.yml`) or delete the duplicate job names so the unused file cannot drift back into same-name publication.

### Unused reusable workflow

`.github/workflows/repository-policy.yml` is not the live `pull_request_target` path. Do not wire it. If it remains in-tree, give it the same one-publisher semantics or remove the two same-name jobs. Do not leave alignment tests requiring `needs: [revoke, contract]` on that file.

## Issue #5 / PR #7 recovery order

Do not mutate Issue #5, PR #7, Go sources, or this plan-only branch's non-plan files during implementation authoring of those foreign artifacts.

1. Merge this plan-only pull request only after owner `repo-ops.plan-approval.v1` naming this file's plan digest and plan commit. This draft is not approval, merge, release, or ruleset authority.
2. Open a separate high-risk implementation pull request, `Related #8`, `Changelog: required`, with fragment `changelog.d/8-repair-policy-publication.md`. Implementation round limit remains `5`.
3. **Maintainer GitHub edit, not a repository file:** change Issue #5's `## Completion criteria` heading to `## Acceptance`. The v0.1.0 contract requires `## Acceptance`; the diagnostic is `linked issue body is missing ## Acceptance`. Perform this edit before claiming contract green on PR #7. Do not edit Issue #5 from the plan-only pull request.
4. Squash-merge the implementation pull request using repository defaults `PR_TITLE` + `BLANK`. Do not change rulesets. Do not tag or release.
5. After the hotfix is on `main`, rebase PR #7 (`plan/5-go-validator`) onto the new `origin/main`. Do not rewrite PR #7's plan body except as required by rebase conflict resolution of the single plan file. Update PR #7's frozen-base SHA only if that plan's authoring SHA sentence would otherwise name a superseded `main`.
6. On the rebased PR #7 head, verify actual Checks API contexts (commands below). Then Issue #5 may proceed under its own plan. Do not start Go from Issue #8.

## Execution

Implementation (not this pull request) touches, and only:

- `actions/repository-policy/action.yml` (quote the scalar)
- `.github/workflows/policy.yml` (one `publish` job)
- `.github/workflows/repository-policy.yml` (remove same-name jobs or align)
- `.github/repo-policy.yml` (`quality.check: quality`)
- `contracts/v0.1.0/repository-policy.schema.json` (`const: "quality"`)
- `fixtures/valid-high.json`, `fixtures/valid-low-direct.json`
- `tests/test_validator.py` (alignment + manifest type test)
- `changelog.d/8-repair-policy-publication.md`

Do not edit `validate.py` collector algorithm unless a test proves it still compares against `CI / quality` after the policy/schema change (it reads `policy.quality.check`; it should follow the YAML change with no code edit). Do not edit `ci.yml` job name. Do not add ruleset JSON.

## Commands

Working directory is the repository root. Implementation branch only; this plan-only pull request does not run them.

Quote-check the scalar (must be silent after the fix; must fail on `main` today if the unquoted line is grepped as a quoted requirement):

```text
python -c "from pathlib import Path; text = Path('actions/repository-policy/action.yml').read_text(encoding='utf-8'); assert 'cache-dependency-glob: \"${{ github.action_path }}/uv.lock\"' in text"
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

Live composite load evidence: the `publish` job must get past `uses: ./.repo-ops/actions/repository-policy` without GitHub's composite-manifest parse error. A red `contract` check from policy findings is acceptable; a job failure with “Unable to process action”, invalid `action.yml`, or unexpected `with` type is not.

Issue #5 heading (maintainer, after or in parallel with implementation, before PR #7 contract can pass):

```text
gh issue view 5 --repo haesol-shin/.github --json body --jq .body | python -c "import sys; b=sys.stdin.read(); assert '## Acceptance' in b; assert '## Completion criteria' not in b"
```

Rebase PR #7 after hotfix merge:

```text
git fetch origin
git checkout plan/5-go-validator
git rebase origin/main
git push --force-with-lease
```

## Verification and recovery

### Plan-only pull request (this change)

Diff must contain only `.ops/plans/8-repair-policy-publication.md`. No workflow, action, schema, fixture, test, changelog, Issue, or PR #7 mutation.

Expected live checks on this exact head against unrepaired `main`:

- `quality` — pass if unittest discovery ignores `.ops/plans/`.
- `contract` / `merge approval` — may be missing, duplicated, or fail closed because the merged publisher is broken. This draft must not claim those contexts are green and must not “fix” them here.

Unverified on this head: unique final `contract` and `merge approval` publication. That evidence is an acceptance gate of the implementation pull request, then again on rebased PR #7.

### Implementation acceptance

- Composite action loads in a real `publish` job.
- Quoted glob is in `action.yml`; manifest unit test fails if the quotes are removed.
- `quality.check` schema const, repo policy, and fixtures equal `quality`.
- `python -m unittest discover -s tests -v` passes, including updated alignment pins.
- Checks API on the implementation head: latest `quality`, `contract`, and `merge approval` exist; no Actions job named `contract` or `merge approval`; publisher job is `publish`.
- Forced evaluator failure (fixture or temporary known-bad PR body) publishes red `contract` and red `merge approval`.
- Forced publication failure after revoke leaves both policy contexts red (not missing-as-pending success).
- Issue #5 heading is `## Acceptance` before PR #7 is required to pass contract.
- After hotfix squash-merge, PR #7 rebases cleanly or only in its plan file, and the same Checks API uniqueness command is recorded on the rebased head.

### Negative cases

- Unquoted `cache-dependency-glob: ${{ github.action_path }}/uv.lock` must fail the manifest test and must not ship.
- `quality.check: CI / quality` must fail schema validation.
- Reintroducing `name: contract` or `name: merge approval` as Actions job names in `policy.yml` must fail alignment tests.
- Wiring `policy.yml` to `repository-policy.yml` remains forbidden.
- Do not add a bypass for Issue #5's heading; repair the Issue body.

### Explicit non-authority

This plan does not authorize: merging without owner plan approval; implementing from this draft; changing rulesets; enabling required checks; tagging or releasing `v0.1.0`; closing Issue #3, #5, or #8 from the plan-only pull request; starting Go; rewriting `main` or PR #6; keeping duplicate same-name Actions jobs.

If implementation review fails, repair that branch. If live publication is still duplicated or the composite action still fails to load, do not rebase PR #7 and do not start Go. Rollback of a merged hotfix is revert of that squash commit (`PR_TITLE` + `BLANK`); do not change rulesets to compensate.
