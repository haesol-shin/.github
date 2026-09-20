# Migrate repository policy validator to Go

Issue: #5
Current v1 intent: `sha256:ab01092464d6f25e1385604c238a0fb4a9eee37aa3c58c912cb616f07e21651c`
Supersedes: `sha256:3c5898cf216583590419b74147ed57bf4af27a762d3ee5a470d93036a6750cb7`
Frozen production oracle: `fe229702e88cda5e1fb7ad142112edb50fd57c82` (`origin/main` after PR #9)

## Approach

Replace the production repository-policy validator with one Go implementation before `repo-ops/v0.1.0` is released. Preserve semantic findings (codes/meaning) and order, exit codes, canonical intent/plan/diff digests, check conclusions, event behavior, and trust boundaries. Machine contexts remain exactly `quality`, `contract`, and `merge approval`. Python is a temporary compatibility oracle only. After zero semantic divergence, delete the Python validator, uv, and validator runtime dependencies in one clean cutover. Rollback is a normal revert to the last Python production commit, not a second live implementation.

Compatibility does **not** freeze Python `jsonschema` or PyYAML diagnostic wording, argparse library text, or stdout/stderr whitespace. Do not require rendered workflow/job labels such as `CI / quality` or `Repository policy / contract`. Do not claim those UI strings as machine contexts.

This plan does not rewrite `contracts/v0.1.0`, change policy outcomes, weaken coverage, migrate other repositories, move semantic reasoning into Go, alter repository rulesets, enable enforcement, tag, release, or adopt consumers. Issue #3 remains the release-train authority and waits for this cutover. Issue #5 pull requests use `Related #5`. Implementation pull requests after this plan-only artifact maintain one `changelog.d/5-go-validator.md` fragment. This plan-only pull request declares `Changelog: not-required`.

PR #7 (this plan-only pull request, rebased onto the PR #9 squash) is the post-merge runtime smoke of the repaired policy. That smoke must pass before Stage 1 starts. Do not start Go if unique latest `quality`, `contract`, and `merge approval` results are missing, if an Actions job is named `contract` or `merge approval`, or if the composite action fails to load from the immutable base.

The merged repository contract is authoritative. Generic Issue/PR heading names in templates and `contract.json` are advisory. Do not reintroduce blocking generic-heading findings or merge-review round counters during the port.

### Frozen current behavior

The production entrypoint is `actions/repository-policy/validate.py`, invoked by `actions/repository-policy/action.yml` through `uv run --project … --locked python … --event "$GITHUB_EVENT_PATH" --check "${{ inputs.check }}"`. CI quality is `.github/workflows/ci.yml` job `quality` (`name: quality`) running `uv run --project actions/repository-policy --locked python -m unittest discover -s tests -v`. Live policy is `.github/workflows/policy.yml` (`pull_request_target`, `pull_request_review`, `issue_comment`) which clones the immutable base SHA, runs the composite action from `.repo-ops/actions/repository-policy`, and publishes check runs named `contract` and `merge approval` on the exact head from one Actions job `publish`. `.github/workflows/repository-policy.yml` is a reusable `workflow_call` entry with the same one-publisher shape; it is not the live central path and must not be called from `policy.yml`.

CLI contract to preserve:

- Exactly one of `--fixture` or `--event` is required; both or neither is an error with exit 2. Library argparse wording is not frozen.
- `--check` choices: `all` (default), `contract`, `merge-approval`.
- Exit 0 iff the findings list is empty; otherwise exit 1. Empty findings publish check conclusion `success`; non-empty publish `failure`.
- Caught live failures (`OSError`, `RuntimeError`, `ValueError`, YAML parse errors) become a single finding and exit 1. Python `str(error)` / PyYAML wording is not frozen.
- For `--check merge-approval`, evaluate contract first; if contract findings exist, insert `contract check did not succeed; merge approval is blocked` at index 0 of the merge-approval findings, then emit merge-approval findings.
- Semantic stdout payload: each finding is the suffix of `::warning title=Repository policy::{finding}`, in evaluator order. Human summary lines, step-summary Markdown, and surrounding whitespace are presentation.

Canonical digest algorithm (authoritative, language-independent; exact bytes):

1. `normalize_text(value)` replaces `\r\n` and `\r` with `\n`, `rstrip()`s, then appends exactly one trailing `\n`.
2. `canonical_digest(payload)` is `sha256:` plus hex of SHA-256 over UTF-8 `json.dumps(payload, ensure_ascii=False, sort_keys=True, separators=(",", ":"))`.
3. Intent v1 digest payload is `{"issue": <int>, "outcome": normalize_text(outcome), "risk": <str>}`.
4. Legacy intent digest payload is `{"outcome": normalize_text(outcome), "risk": <str>}`.
5. Plan digest is `canonical_digest({"text": normalize_text(plan_bytes_decoded_as_utf8)})`.
6. Diff digest is SHA-256 of the exact stdout byte stream of external git (not go-git, not libgit2):

```text
git -c core.quotePath=true diff --binary --full-index --no-color --no-ext-diff --no-textconv --no-renames refs/repo-ops/base refs/repo-ops/head --
```

The Go collector must reproduce `git_metadata`: temp dir prefix `repo-ops-`, `git init --quiet`, fetch `refs/pull/{number}/head:refs/repo-ops/head` with `GIT_CONFIG_COUNT=1` `http.https://github.com/.extraheader=AUTHORIZATION: basic <base64(x-access-token:{token})>` when a token is present, require fetched HEAD to equal the GitHub API head SHA, `git cat-file -e {merge_base}^{commit}`, `git update-ref refs/repo-ops/base {merge_base}`, hash stdout in 1 MiB reads, fail on non-zero git status. Do not hash stderr. Do not rewrite diff text.

GitHub collection to preserve:

- `GITHUB_TOKEN` required for live mode; `GITHUB_API_URL` default `https://api.github.com`.
- User-Agent `repo-ops-validator/0.1`, `X-GitHub-Api-Version: 2022-11-28`, `Authorization: Bearer {token}`, 30s timeout.
- Pagination follows `Link` `rel="next"`; list endpoints use `per_page=100`; check-runs use `field=check_runs`.
- Policy YAML is loaded from immutable **base** SHA path `.github/repo-policy.yml`.
- Added/modified fragment bytes are loaded from **head** SHA (data only).
- Plan bytes for the current digest are loaded from **head** SHA; plan bytes for an approval commit are loaded from that commit; a plan-commit is accepted only when compare `{plan-commit}...{head}` status is `ahead` or `identical` and both digests equal the approval `plan` field.
- Quality success is the last check-run whose `name` equals `policy.quality.check` (currently `quality`) and whose `conclusion` is `success`. Do not remap that string to a rendered `CI / quality` label during the port.
- Reviews with empty body or `state == DISMISSED` are ignored.
- Trusted code, schemas, and the action checkout are the immutable base SHA. Pull-request code is never executed.

Evaluator-authored finding strings and order are frozen by `tests/test_validator.py` plus `fixtures/*.json` as semantic identity (codes/meaning). Schema and YAML **outcomes** are frozen (same instances fail or pass, same order relative to other findings); Python `jsonschema==4.25.1` and PyYAML 6.0.2 diagnostic wording is not. A general JSON Schema library is allowed. Compare schema failures by schema, instance path, and required-field/code identity, not by library message text.

Generic Issue/PR headings are advisory: `validate_heading_contract` returns no findings. Templates and `contract.json` heading lists remain documentation. Changelog declarations, authority lines, risk lines, and fenced machine records remain blocking. `repo-ops.merge-review.v1` field order is `verdict risk intent plan base head diff runtime model` with no `plan-round` or `implementation-round`.

`changelog.py` is not the trusted live validator, but CI currently loads it through the same unittest module and `validate.py` imports `parse_fragment` for fragment body checks. Cutover deletes uv and the validator Python path; folding therefore moves into Go in stage 3 so no Python/uv validator setup remains. Until cutover, Python folding remains the oracle.

Issue #5 already uses `## Acceptance`. This plan does not edit the Issue. Missing or reordered generic headings are not merge blockers.

### Package and command boundaries

One Go module at the repository root, module path `github.com/haesol-shin/.github`, `CGO_ENABLED=0`. Pin the toolchain in `go.mod` during stage 1 to a stable upstream series installable on `ubuntu-latest` without extra apt sources. No additional production bootstrap (no uv, no pip, no container image beyond `ubuntu-latest` + official Go setup).

| Path | Responsibility |
| --- | --- |
| `cmd/repo-ops-validator` | CLI parity with `validate.py` `main()`: `--event`, `--fixture`, `--check` |
| `cmd/repo-ops-changelog` | CLI parity with `changelog.py`: `--root`, `--changelog`, `--version`, `--date` (land by stage 3; may exist earlier as oracle-tested dead code not wired to production) |
| `internal/canonical` | `normalize_text`, `canonical_digest`, `plan_digest` |
| `internal/evaluate` | authority lines, records, intent chain, risk authority, changelog state, `validate_state`; headings remain advisory |
| `internal/schema` | Draft 2020-12 evaluation; failures compared by schema/path/code, not Python message text |
| `internal/collect` | `GitHubClient`, `build_live_state`, `api_content`, `git_metadata`, collaborator permissions |
| `internal/fixture` | `load_fixture` including `extends` / dotted `replace` |

Composite action remains `actions/repository-policy/action.yml`. Production wiring changes only in stage 3: build the validator from the immutable base checkout and exec the binary with the same `--event` / `--check` flags. Do not add a second composite action or a second check name.

Rejected alternatives:

- Dual production Python+Go path after cutover.
- Embedding CPython, PyOxidizer, or WASM-Python.
- Replacing external git with go-git/git2go (byte stream would drift).
- Renaming machine contexts or collapsing `contract` and `merge approval`.
- Treating rendered labels `CI / quality` or `Repository policy / …` as required contexts.
- Changing evaluator-authored finding text to be “more idiomatic”.
- Freezing `jsonschema` / PyYAML diagnostic strings or stdout/stderr whitespace as cutover gates.
- Reintroducing merge-review round counters or blocking generic-heading findings.
- New production dependency manager or container bootstrap.

Dependencies: standard library for HTTP, git subprocess, JSON, SHA-256, and CLI. YAML parser only if it yields the same policy object as the frozen `.github/repo-policy.yml` data model (today: scalars, nested maps, one list). Prefer `gopkg.in/yaml.v3` only after a fixture proves equivalent `policy` objects. Vendor or `go.mod` sums are required before cutover. No `jsonschema`/`pyyaml`/uv in production after cutover.

### Three reviewable implementation stages

All stages are separate `Related #5` pull requests against the then-current `main`. High-risk plan approval on each implementation pull request names this plan digest and the plan commit that introduced it (or a later reviewed plan commit if this file changes). There is no plan-round or implementation-round limit.

**Stage 1 — freeze and fixture evaluator.** Export language-independent semantic golden artifacts from the Python oracle at `fe229702e88cda5e1fb7ad142112edb50fd57c82` (rebase the freeze if `main` later contains only non-validator commits; never silently accept oracle drift). Land `cmd/repo-ops-validator --fixture` plus `internal/{canonical,evaluate,schema,fixture}` with no GitHub or git collection. Add the semantic differential harness. Production workflows stay Python. Changelog fragment `changelog.d/5-go-validator.md` is created here (`Changelog: required`). Do not start this stage until PR #7 policy smoke has passed.

**Stage 2 — collector, replay, shadow, benchmarks.** Implement `internal/collect` and live `--event`. Add sanitized supported-event replay and shadow comparison against Python on the same event payload. Record benchmarks. Production workflows still call Python. Repair every semantic divergence; do not cut over.

**Stage 3 — clean cutover.** Point `action.yml`, `ci.yml`, and `policy.yml` at the Go binary built from the immutable base. Delete the Python validator path, uv, and runtime dependencies. Port folding to `cmd/repo-ops-changelog`. Update `repo-policy.yml` `quality.commands` to the Go test invocation while keeping `quality.check: quality`. No dual path, no extra bootstrap.

## Execution

1. Merge this plan-only pull request only after (a) owner `repo-ops.plan-approval.v1` naming this file’s plan digest and plan commit, and (b) PR #7 post-merge policy smoke on this exact head has passed. Do not start Go code before both. Do not treat this draft as approval, merge, release, or ruleset authority.
2. **PR #7 policy smoke** (gates Stage 1; this pull request is the vehicle). On exact head `$HEAD`:

```text
gh api --paginate "repos/haesol-shin/.github/commits/${HEAD}/check-runs?per_page=100" \
  --jq '.check_runs | group_by(.name) | map({name: .[0].name, n: length, latest: (sort_by(.id) | last | {conclusion, started_at, id})})'
```

   Accept: latest `quality` is `success` when CI passed; latest `contract` and `merge approval` are custom results; no Actions job named `contract` or `merge approval`; an Actions job named `publish` may appear; no check-run name `CI / quality`; the `publish` job log gets past `uses: ./.repo-ops/actions/repository-policy` without a YAML/`action.yml` parse error. Revoke-then-final may yield `n >= 2` API rows per policy context. If smoke fails, do not start Go; repair against `main` or revert the PR #9 squash. Do not change rulesets to compensate.
3. **Stage 1 freeze.** From repository root, using the Python oracle at the frozen SHA, write golden files under `.ops/evidence/5-go-validator/oracle/` (gitignored working copy is acceptable during authoring; committed goldens live under `fixtures/` and `tests/testdata/go-oracle/`):
   - For every `fixtures/*.json` except `changelog-conformance.json` and `intent-comment-revocation.json`, run `--check all`, `--check contract`, and `--check merge-approval`.
   - Capture `{exit, findings, check_conclusion}` where `findings` is every `::warning title=Repository policy::` suffix in order, and `check_conclusion` is `success` iff `exit == 0` else `failure`. Do not treat full stdout/stderr bytes as goldens.
   - Capture `canonical_digest` / `plan_digest` vectors from `tests/test_validator.py` intent and plan cases (exact digest bytes).
   - Capture schema-failure identity by feeding each frozen schema an empty object and each required-field omission through `validate_schema`: record schema label, instance path, and required-field/code, not Python `error.message`.
   Commit goldens with the Go evaluator. `go test ./internal/... ./cmd/repo-ops-validator` must pass without network.
4. **Stage 1 differential command** (semantic; fail on mismatch of exit, ordered finding payloads, conclusions, and canonical digests — not on stdout/stderr whitespace):

```text
python actions/repository-policy/validate.py --fixture "$FIXTURE" --check "$CHECK" >"$OUT/python.stdout" 2>"$OUT/python.stderr"; echo $? >"$OUT/python.exit"
go run ./cmd/repo-ops-validator --fixture "$FIXTURE" --check "$CHECK" >"$OUT/go.stdout" 2>"$OUT/go.stderr"; echo $? >"$OUT/go.exit"
python -c "from pathlib import Path
import json, re, sys
def art(prefix):
    out = Path('$OUT', prefix + '.stdout').read_text(encoding='utf-8', errors='replace')
    err = Path('$OUT', prefix + '.stderr').read_text(encoding='utf-8', errors='replace')
    exit_code = int(Path('$OUT', prefix + '.exit').read_text().strip())
    findings = re.findall(r'(?m)^::warning title=Repository policy::(.*)$', out)
    return {'exit': exit_code, 'findings': findings, 'check_conclusion': 'success' if exit_code == 0 else 'failure', 'token_leaked': 'GITHUB_TOKEN' in out+err}
a, b = art('python'), art('go')
sys.exit(0 if a == b else 1)"
```

`$FIXTURE` is every file in `fixtures/*.json` that `load_fixture` accepts as a validator state (`changelog-conformance.json` is folding-only; `intent-comment-revocation.json` is workflow-only). `$CHECK` is `all`, `contract`, and `merge-approval`. Working directory is the repository root. `GITHUB_STEP_SUMMARY` is unset during comparison so summary presentation cannot mask finding payloads. Do not `cmp` stdout, stderr, or step-summary files.

5. **Stage 1 Go package rules.** `internal/evaluate` must not import `net/http` or `os/exec`. `internal/collect` must not be referenced from fixture tests. Finding order equals Python append order in `validate_state` / `validate_changelog_state` for evaluator-authored findings.
6. **Stage 2 collector.** Implement live `--event` with the frozen HTTP, pagination, base-owned policy load, head-owned fragment/plan load, permission lookup, and external-git hashing. Unit-test collectors with recorded HTTP fixtures (httptest), not the live API, plus one optional shadow job.
7. **Stage 2 supported-event replay inputs.** Sanitize real `policy.yml` deliveries into `tests/testdata/events/` with tokens, authorization headers, and extraheader values replaced by `REDACTED`. Required event files (minimum set; add rather than drop):

| File | `github.event_name` | `action` | Purpose |
| --- | --- | --- | --- |
| `pull_request_target-opened.json` | `pull_request_target` | `opened` | first contract+merge-approval publication |
| `pull_request_target-edited.json` | `pull_request_target` | `edited` | body/title authority change |
| `pull_request_target-reopened.json` | `pull_request_target` | `reopened` | re-open |
| `pull_request_target-synchronize.json` | `pull_request_target` | `synchronize` | exact-head change |
| `pull_request_review-submitted.json` | `pull_request_review` | `submitted` | merge-review/plan-approval on review |
| `pull_request_review-edited.json` | `pull_request_review` | `edited` | stale review body |
| `pull_request_review-dismissed.json` | `pull_request_review` | `dismissed` | dismissed reviews ignored by collector |
| `issue_comment-created-pr.json` | `issue_comment` | `created` | PR comment with plan-approval or merge-review |
| `issue_comment-edited-pr.json` | `issue_comment` | `edited` | approval edit revocation |
| `issue_comment-deleted-pr.json` | `issue_comment` | `deleted` | approval deletion revocation |
| `issue_comment-created-intent.json` | `issue_comment` | `created` | Issue (non-PR) intent comment; workflow revocation only |
| `issue_comment-edited-intent.json` | `issue_comment` | `edited` | intent supersession |
| `issue_comment-deleted-intent.json` | `issue_comment` | `deleted` | intent deletion |

Replay command (capture the same semantic artifact as Stage 1; do not `cmp` stdout/stderr):

```text
GITHUB_TOKEN=replaying GITHUB_REPOSITORY=haesol-shin/.github \
  python actions/repository-policy/validate.py --event "$EVENT" --check "$CHECK" >"$OUT/python.stdout" 2>"$OUT/python.stderr"; echo $? >"$OUT/python.exit"
GITHUB_TOKEN=replaying GITHUB_REPOSITORY=haesol-shin/.github \
  go run ./cmd/repo-ops-validator --event "$EVENT" --check "$CHECK" >"$OUT/go.stdout" 2>"$OUT/go.stderr"; echo $? >"$OUT/go.exit"
```

For collector unit tests, HTTP and git are stubbed. For shadow comparison on a real pull request, run both binaries against the same `$GITHUB_EVENT_PATH` with the same `GITHUB_TOKEN` and compare the semantic artifact (exit, ordered finding payloads, conclusions, canonical digests). Shadow is advisory evidence, not a production check name. Token values must be absent from stdout, stderr, and step summary (exact non-disclosure).

8. **Stage 2 immutable-base trust checks** (must fail closed in tests):

- Validator binary, `action.yml`, schemas, and `.github/repo-policy.yml` used by production come from the base SHA checkout under `.repo-ops`, never from `pull_request.head`.
- A fixture where head modifies `actions/repository-policy/**` or `contracts/**` does not change findings relative to base-owned copies.
- `GITHUB_TOKEN` is not printed to stdout, stderr, or step summary.
- Workflow permissions remain `contents: read`, `issues: read`, `pull-requests: read`, `checks: write` on policy jobs; the Go binary does not request scopes.
- Executing PR-head Python/Go is a failed trust test, not a feature.

9. **Stage 2 external-git byte hashing evidence.** For a known merge-base/head pair:

```text
git -c core.quotePath=true diff --binary --full-index --no-color --no-ext-diff --no-textconv --no-renames "$BASE" "$HEAD" -- | sha256sum
```

The validator `diff_digest` must equal `sha256:` plus that hex. Include a binary file, a path needing `core.quotePath`, a deletion, and an empty diff. Go-git hashes are not evidence.

10. **Stage 2 benchmark evidence** (record, do not gate cutover). Write `tests/testdata/benchmarks/5-go-validator.json` with this schema:

```json
{
  "oracle_commit": "<40-hex>",
  "go_commit": "<40-hex>",
  "local_cold_start_ms": {"python": 0, "go": 0, "n": 0},
  "clean_ci_wall_ms": {"python": 0, "go": 0, "p50": 0, "p95": 0, "n": 0},
  "peak_rss_kib": {"python": 0, "go": 0},
  "workflow_steps": {"python": 0, "go": 0},
  "production_dependencies": {"python": ["jsonschema==4.25.1", "pyyaml==6.0.2", "uv"], "go": ["<module>@<version>", "..."]},
  "explanations": []
}
```

Commands: local cold start is the wall time of a fresh process `--fixture fixtures/valid-high.json --check all` after dropping OS file cache when practical; n ≥ 5. Peak RSS is max `ru_maxrss` / equivalent. Clean CI wall is the `quality` job duration on an empty-cache workflow_run. Workflow step count is production `policy.yml` steps on `pull_request_target` synchronize. Dependency count is production trusted-path modules only. Regressions require an `explanations[]` entry; they do not retain Python.

11. **Stage 3 cutover.** In one pull request: build `repo-ops-validator` from the base checkout in `action.yml` and `policy.yml`; replace CI `uv run … unittest` with `go test ./...`; change `quality.commands` to that Go test line; wire `cmd/repo-ops-changelog` for release folding; delete the Python inventory below; keep check publication names `contract` and `merge approval`. After merge, verify post-merge `quality` on the exact `main` SHA.

12. **Deletion inventory (stage 3, complete cutover):**

- `actions/repository-policy/validate.py`
- `actions/repository-policy/changelog.py`
- `actions/repository-policy/pyproject.toml`
- `actions/repository-policy/uv.lock`
- `actions/repository-policy/.python-version`
- `astral-sh/setup-uv@37802adc94f370d6bfd71619e3f0bf239e1f3b78` from `action.yml`, `ci.yml`, `policy.yml`, and `repository-policy.yml`
- `uv run --project … python …` production steps
- Python-only unittest runner from CI (tests themselves move to `go test`)

Do not delete `contracts/`, `fixtures/` JSON states, `.github/repo-policy.yml`, or check publication logic. `tests/test_validator.py` is deleted only when Go tests cover the same observable contracts.

13. **Post-cutover verification.** Replay every frozen fixture and every supported event with the Go binary only. Confirm check-run names remain `quality`, `contract`, and `merge approval`. Confirm `Related #5` still does not close Issue #5. Do not tag `v0.1.0`, protect `v*`, publish a GitHub Release, change rulesets, or close Issue #3.

14. **Rollback.** Revert the stage 3 squash commit (or the contiguous cutover commits) to restore the last Python production commit. Do not land a feature flag or parallel action. Stages 1–2 rollback is likewise revert; Python remains production until stage 3 merges.

## Verification and recovery

### Plan-only pull request (this change)

The diff must contain only `.ops/plans/5-migrate-repository-policy-validator-to-go.md`. No Go code, fixtures, changelog fragments, workflows, tests, or contract edits. This pull request is the PR #7 runtime smoke of the repaired policy. Expected live checks on the merged base:

- `quality` — pass if the plan file does not affect unittest discovery.
- `contract` — fail closed until owner plan approval exists.
- `merge approval` — fail closed unless contract succeeded and an exact-head `repo-ops.merge-review.v1` exists.

Those publications are the proof that the repaired base-owned contexts exist. Do not interpret a failed contract on this draft as permission to change the contract or skip later stages. Owner plan approval and passing PR #7 smoke are gates on merging this plan and on starting Go, not Go work itself.

### Stage 1 acceptance

- Semantic artifact comparison is silent for every fixture × check matrix: matching exit codes, ordered finding payloads, and check conclusions.
- Intent/plan canonical digests match Python for the frozen vectors (exact digest bytes).
- Schema goldens match on schema/path/code identity, not Python message text.
- `internal/evaluate` has no network or git.
- Production workflows still invoke Python.

### Stage 2 acceptance

- Replay matrix has zero Python/Go divergence on semantic findings, order, exit codes, check conclusions, and plan/diff digests.
- Trust tests prove base-owned code load and non-execution of head code.
- External-git byte hashes match `sha256sum` of the specified `git diff` stream.
- Token bytes never appear in stdout, stderr, or step summary.
- Benchmark JSON is committed with raw numbers; unexplained regressions block the stage 2 merge, but explained regressions do not retain Python.

### Stage 3 acceptance

- Production path runs one Go binary from immutable base.
- Deletion inventory is empty on `main`.
- No `uv`, `jsonschema`, `pyyaml`, or `validate.py`.
- Machine contexts remain `quality`, `contract`, and `merge approval`.
- Rollback drill documented as `git revert` of the cutover squash.

### Negative cases (must remain failing with the same semantic findings)

Invalid fixtures already on `main`: `invalid-missing-field.json`, `invalid-diff-digest.json`, `invalid-stale-head.json`, `invalid-direct-changelog.json`, `invalid-fragment-deletion.json`, `invalid-fragment-owner.json`, `invalid-fragment-section.json`, `invalid-required-missing.json`. Additional evaluator negatives already covered by `tests/test_validator.py`: fenced false authority, legacy-after-v1, broken supersession, non-owner high-risk plan approval, shared-identity vs native review, argparse both/neither input (exit 2), merge-approval blocked by contract, fetched HEAD mismatch, missing `GITHUB_TOKEN`, unsupported content encoding, paginated non-list, and noncanonical record spacing. Schema required-field omissions must still fail; the Python `jsonschema` sentence is not the identity.

Must **not** fail: missing, duplicate, or reordered generic Issue/PR headings (advisory). Must **not** require `plan-round` / `implementation-round` on merge-review receipts. Reintroducing those round fields must fail schema/field-order tests. `quality.check: CI / quality` must fail schema validation.

### Explicit non-authority

This plan does not authorize: merging without owner plan approval; starting Go before PR #7 policy smoke passes; changing rulesets; enabling required checks; tagging or releasing `v0.1.0`; closing Issue #3 or Issue #5; adopting `repo-ops/v0.1.0` in a consumer; keeping Python as a production fallback after stage 3.

If stage 1, 2, or 3 review fails, repair that branch. If compatibility or trust diverges, fix Go until the semantic comparison artifact is identical. If cutover CI fails, revert the cutover commit. If a defect is found after `v0.1.0` publication (a later Issue #3 gate), do not rewrite that tag.
