# Migrate repository policy validator to Go

Issue: #5
Current v1 intent: `sha256:3c5898cf216583590419b74147ed57bf4af27a762d3ee5a470d93036a6750cb7`
Frozen production oracle: `08f456934179cf641d5f66220a2cf397f62ba2a6` (`origin/main` at plan authoring)

## Approach

Replace the production repository-policy validator with one Go implementation before `repo-ops/v0.1.0` is released. Preserve every public contract, finding string, finding order, digest, exit code, stdout/stderr contract, trust boundary, and required-check context. Python is a temporary compatibility oracle only. After zero divergence, delete the Python validator, uv, and validator runtime dependencies in one clean cutover. Rollback is a normal revert to the last Python production commit, not a second live implementation.

This plan does not rewrite `contracts/v0.1.0`, change policy outcomes, weaken coverage, migrate other repositories, move semantic reasoning into Go, alter repository rulesets, enable enforcement, tag, release, or adopt consumers. Issue #3 remains the release-train authority and waits for this cutover. Issue #5 pull requests use `Related #5`. Implementation pull requests after this plan-only artifact maintain one `changelog.d/5-go-validator.md` fragment. This plan-only pull request declares `Changelog: not-required`.

Machine contexts remain `quality`, `contract`, and `merge approval`. Rendered workflow/job labels remain `CI / quality`, `Repository policy / contract`, and `Repository policy / merge approval`. The merged repository contract is authoritative. Do not “fix” label/context spelling during the port.

### Frozen current behavior

The production entrypoint is `actions/repository-policy/validate.py`, invoked by `actions/repository-policy/action.yml` through `uv run --project … --locked python … --event "$GITHUB_EVENT_PATH" --check "${{ inputs.check }}"`. CI quality is `.github/workflows/ci.yml` job `quality` running `uv run --project actions/repository-policy --locked python -m unittest discover -s tests -v`. Live policy is `.github/workflows/policy.yml` (`pull_request_target`, `pull_request_review`, `issue_comment`) which clones the immutable base SHA, runs the composite action from `.repo-ops/actions/repository-policy`, and publishes check runs named `contract` and `merge approval` on the exact head. `.github/workflows/repository-policy.yml` is a reusable workflow that is not the live `pull_request_target` path; alignment tests forbid `uses: ./.github/workflows/repository-policy.yml` from `policy.yml`.

CLI contract to preserve:

- Exactly one of `--fixture` or `--event` is required; both or neither is an argparse error (exit 2, stderr).
- `--check` choices: `all` (default), `contract`, `merge-approval`.
- Exit 0 iff the findings list is empty; otherwise exit 1.
- Caught live failures (`OSError`, `RuntimeError`, `ValueError`, `yaml.YAMLError`) become a single finding equal to `str(error)`.
- For `--check merge-approval`, evaluate contract first; if contract findings exist, insert `contract check did not succeed; merge approval is blocked` at index 0 of the merge-approval findings, then emit merge-approval findings.
- `emit_findings` writes each finding as `::warning title=Repository policy::{finding}` to stdout, appends `## Repository policy / {check} findings` plus bullets to `$GITHUB_STEP_SUMMARY` when set, then prints `Repository policy / {check}: {n} finding(s)` or `Repository policy / {check}: contract satisfied`. `{check}` is the raw `--check` value (`all`, `contract`, or `merge-approval`).

Canonical digest algorithm (authoritative, language-independent):

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
- Quality success is the last check-run whose `name` equals `policy.quality.check` (currently `CI / quality`) and whose `conclusion` is `success`. Do not remap that string to the job id `quality` during the port.
- Reviews with empty body or `state == DISMISSED` are ignored.
- Trusted code, schemas, and the action checkout are the immutable base SHA. Pull-request code is never executed.

Evaluator finding strings and order are frozen by `tests/test_validator.py` plus `fixtures/*.json`. Schema diagnostics are `"{label}: {jsonschema.Draft202012Validator message}"` from Python `jsonschema==4.25.1`. Go must match those strings; a general JSON Schema library is allowed only if every frozen schema diagnostic is byte-identical. Otherwise implement a focused adapter over the five frozen schemas.

`changelog.py` is not the trusted live validator, but CI currently loads it through the same unittest module and `validate.py` imports `parse_fragment` for fragment body checks. Cutover deletes uv and the validator Python path; folding therefore moves into Go in stage 3 so no Python/uv validator setup remains. Until cutover, Python folding remains the oracle.

Live Issue #5 currently titles the acceptance section `## Completion criteria` while `contracts/v0.1.0/contract.json` requires `## Acceptance`. This plan does not edit the Issue. The first base-owned contract run against this plan-only pull request is expected to include `linked issue body is missing ## Acceptance` until a maintainer edits the Issue body. That edit is maintainer-owned, not an implementation mutation.

### Package and command boundaries

One Go module at the repository root, module path `github.com/haesol-shin/.github`, `CGO_ENABLED=0`. Pin the toolchain in `go.mod` during stage 1 to a stable upstream series installable on `ubuntu-latest` without extra apt sources. No additional production bootstrap (no uv, no pip, no container image beyond `ubuntu-latest` + official Go setup).

| Path | Responsibility |
| --- | --- |
| `cmd/repo-ops-validator` | CLI parity with `validate.py` `main()`: `--event`, `--fixture`, `--check` |
| `cmd/repo-ops-changelog` | CLI parity with `changelog.py`: `--root`, `--changelog`, `--version`, `--date` (land by stage 3; may exist earlier as oracle-tested dead code not wired to production) |
| `internal/canonical` | `normalize_text`, `canonical_digest`, `plan_digest` |
| `internal/evaluate` | heading contract, authority lines, records, intent chain, risk authority, changelog state, `validate_state` |
| `internal/schema` | frozen Draft 2020-12 evaluation with pinned diagnostic strings |
| `internal/collect` | `GitHubClient`, `build_live_state`, `api_content`, `git_metadata`, collaborator permissions |
| `internal/fixture` | `load_fixture` including `extends` / dotted `replace` |

Composite action remains `actions/repository-policy/action.yml`. Production wiring changes only in stage 3: build the validator from the immutable base checkout and exec the binary with the same `--event` / `--check` flags. Do not add a second composite action or a second check name.

Rejected alternatives:

- Dual production Python+Go path after cutover.
- Embedding CPython, PyOxidizer, or WASM-Python.
- Replacing external git with go-git/git2go (byte stream would drift).
- Renaming machine contexts or collapsing `contract` and `merge approval`.
- Changing finding text to be “more idiomatic”.
- Broad JSON Schema runtime unless diagnostics match `jsonschema==4.25.1` exactly.
- New production dependency manager or container bootstrap.

Dependencies: standard library for HTTP, git subprocess, JSON, SHA-256, and CLI. YAML parser only if it round-trips `.github/repo-policy.yml` identically to PyYAML 6.0.2 for the frozen policy file (today: scalars, nested maps, one list). Prefer `gopkg.in/yaml.v3` only after a fixture proves identical `policy` objects. Vendor or `go.mod` sums are required before cutover. No `jsonschema`/`pyyaml`/uv in production after cutover.

### Three reviewable implementation stages

All stages are separate `Related #5` pull requests against the then-current `main`. High-risk plan approval on each implementation pull request names this plan digest and the plan commit that introduced it (or a later reviewed plan commit if this file changes). Round limits remain `plan 5 / implementation 5`.

**Stage 1 — freeze and fixture evaluator.** Export language-independent golden artifacts from the Python oracle at `08f456934179cf641d5f66220a2cf397f62ba2a6` (rebase the freeze if `main` later contains only non-validator commits; never silently accept oracle drift). Land `cmd/repo-ops-validator --fixture` plus `internal/{canonical,evaluate,schema,fixture}` with no GitHub or git collection. Add the differential harness. Production workflows stay Python. Changelog fragment `changelog.d/5-go-validator.md` is created here (`Changelog: required`).

**Stage 2 — collector, replay, shadow, benchmarks.** Implement `internal/collect` and live `--event`. Add sanitized supported-event replay and shadow comparison against Python on the same event payload. Record benchmarks. Production workflows still call Python. Repair every divergence; do not cut over.

**Stage 3 — clean cutover.** Point `action.yml`, `ci.yml`, and `policy.yml` at the Go binary built from the immutable base. Delete the Python validator path, uv, and runtime dependencies. Port folding to `cmd/repo-ops-changelog`. Update `repo-policy.yml` `quality.commands` to the Go test invocation while keeping `quality.check: CI / quality`. No dual path, no extra bootstrap.

## Execution

1. Merge this plan-only pull request only after owner `repo-ops.plan-approval.v1` naming this file’s plan digest and plan commit. Do not start Go code before that approval. Do not treat this draft as approval, merge, release, or ruleset authority.
2. **Stage 1 freeze.** From repository root, using the Python oracle at the frozen SHA, write golden files under `.ops/evidence/5-go-validator/oracle/` (gitignored working copy is acceptable during authoring; committed goldens live under `fixtures/` and `tests/testdata/go-oracle/`):
   - For every `fixtures/*.json` except `changelog-conformance.json` and `intent-comment-revocation.json`, run both `--check all`, `--check contract`, and `--check merge-approval`.
   - Capture `{stdout,stderr,exit}` plus parsed findings (every `::warning title=Repository policy::` suffix, in order).
   - Capture `canonical_digest` / `plan_digest` vectors from `tests/test_validator.py` intent and plan cases.
   - Capture schema diagnostics by feeding each frozen schema an empty object and each required-field omission through Python `validate_schema`.
   Commit goldens with the Go evaluator. `go test ./internal/... ./cmd/repo-ops-validator` must pass without network.
3. **Stage 1 differential command** (exact; fail on any mismatch):

```text
python actions/repository-policy/validate.py --fixture "$FIXTURE" --check "$CHECK" >"$OUT/python.stdout" 2>"$OUT/python.stderr"; echo $? >"$OUT/python.exit"
go run ./cmd/repo-ops-validator --fixture "$FIXTURE" --check "$CHECK" >"$OUT/go.stdout" 2>"$OUT/go.stderr"; echo $? >"$OUT/go.exit"
cmp "$OUT/python.exit" "$OUT/go.exit"
cmp "$OUT/python.stdout" "$OUT/go.stdout"
cmp "$OUT/python.stderr" "$OUT/go.stderr"
```

`$FIXTURE` is every file in `fixtures/*.json` that `load_fixture` accepts as a validator state (`changelog-conformance.json` is folding-only; `intent-comment-revocation.json` is workflow-only). `$CHECK` is `all`, `contract`, and `merge-approval`. Working directory is the repository root. `GITHUB_STEP_SUMMARY` is unset during cmp so summary side effects cannot mask stdout drift; a separate case sets a temp summary path and cmps those files too.

4. **Stage 1 Go package rules.** `internal/evaluate` must not import `net/http` or `os/exec`. `internal/collect` must not be referenced from fixture tests. Finding order equals Python append order in `validate_state` / `validate_changelog_state` / `validate_heading_contract`.
5. **Stage 2 collector.** Implement live `--event` with the frozen HTTP, pagination, base-owned policy load, head-owned fragment/plan load, permission lookup, and external-git hashing. Unit-test collectors with recorded HTTP fixtures (httptest), not the live API, plus one optional shadow job.
6. **Stage 2 supported-event replay inputs.** Sanitize real `policy.yml` deliveries into `tests/testdata/events/` with tokens, authorization headers, and extraheader values replaced by `REDACTED`. Required event files (minimum set; add rather than drop):

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

Replay command (exact):

```text
GITHUB_TOKEN=replaying GITHUB_REPOSITORY=haesol-shin/.github \
  python actions/repository-policy/validate.py --event "$EVENT" --check "$CHECK" >"$OUT/python.stdout" 2>"$OUT/python.stderr"; echo $? >"$OUT/python.exit"
GITHUB_TOKEN=replaying GITHUB_REPOSITORY=haesol-shin/.github \
  go run ./cmd/repo-ops-validator --event "$EVENT" --check "$CHECK" >"$OUT/go.stdout" 2>"$OUT/go.stderr"; echo $? >"$OUT/go.exit"
```

For collector unit tests, HTTP and git are stubbed. For shadow comparison on a real pull request, run both binaries against the same `$GITHUB_EVENT_PATH` with the same `GITHUB_TOKEN` and cmp stdout/stderr/exit. Shadow is advisory evidence, not a production check name.

7. **Stage 2 immutable-base trust checks** (must fail closed in tests):

- Validator binary, `action.yml`, schemas, and `.github/repo-policy.yml` used by production come from the base SHA checkout under `.repo-ops`, never from `pull_request.head`.
- A fixture where head modifies `actions/repository-policy/**` or `contracts/**` does not change findings relative to base-owned copies.
- `GITHUB_TOKEN` is not printed to stdout, stderr, or step summary.
- Workflow permissions remain `contents: read`, `issues: read`, `pull-requests: read`, `checks: write` on policy jobs; the Go binary does not request scopes.
- Executing PR-head Python/Go is a failed trust test, not a feature.

8. **Stage 2 external-git byte hashing evidence.** For a known merge-base/head pair:

```text
git -c core.quotePath=true diff --binary --full-index --no-color --no-ext-diff --no-textconv --no-renames "$BASE" "$HEAD" -- | sha256sum
```

The validator `diff_digest` must equal `sha256:` plus that hex. Include a binary file, a path needing `core.quotePath`, a deletion, and an empty diff. Go-git hashes are not evidence.

9. **Stage 2 benchmark evidence** (record, do not gate cutover). Write `tests/testdata/benchmarks/5-go-validator.json` with this schema:

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

10. **Stage 3 cutover.** In one pull request: build `repo-ops-validator` from the base checkout in `action.yml` and `policy.yml`; replace CI `uv run … unittest` with `go test ./...`; change `quality.commands` to that Go test line; wire `cmd/repo-ops-changelog` for release folding; delete the Python inventory below; keep check publication names `contract` and `merge approval`. After merge, verify post-merge `CI / quality` on the exact `main` SHA.

11. **Deletion inventory (stage 3, complete cutover):**

- `actions/repository-policy/validate.py`
- `actions/repository-policy/changelog.py`
- `actions/repository-policy/pyproject.toml`
- `actions/repository-policy/uv.lock`
- `actions/repository-policy/.python-version`
- `astral-sh/setup-uv@37802adc94f370d6bfd71619e3f0bf239e1f3b78` from `action.yml`, `ci.yml`, `policy.yml`, and `repository-policy.yml`
- `uv run --project … python …` production steps
- Python-only unittest runner from CI (tests themselves move to `go test`)

Do not delete `contracts/`, `fixtures/` JSON states, `.github/repo-policy.yml`, or check publication logic. `tests/test_validator.py` is deleted only when Go tests cover the same observable contracts.

12. **Post-cutover verification.** Replay every frozen fixture and every supported event with the Go binary only. Confirm check-run names remain `quality`, `contract`, and `merge approval`. Confirm rendered labels remain `CI / quality`, `Repository policy / contract`, and `Repository policy / merge approval`. Confirm `Related #5` still does not close Issue #5. Do not tag `v0.1.0`, protect `v*`, publish a GitHub Release, change rulesets, or close Issue #3.

13. **Rollback.** Revert the stage 3 squash commit (or the contiguous cutover commits) to restore the last Python production commit. Do not land a feature flag or parallel action. Stages 1–2 rollback is likewise revert; Python remains production until stage 3 merges.

## Verification and recovery

### Plan-only pull request (this change)

The diff must contain only `.ops/plans/5-migrate-repository-policy-validator-to-go.md`. No Go code, fixtures, changelog fragments, workflows, tests, or contract edits. Expected live checks on the merged base:

- `CI / quality` (`quality`) — pass if the plan file does not affect unittest discovery.
- `Repository policy / contract` (`contract`) — fail closed until owner plan approval exists, and while Issue #5 lacks `## Acceptance`.
- `Repository policy / merge approval` (`merge approval`) — fail closed unless contract succeeded and an exact-head `repo-ops.merge-review.v1` exists.

Those publications are the proof that the new base-owned contexts exist. Do not interpret a failed contract on this draft as permission to change the contract or skip later stages. Maintainer Issue-body repair and owner plan approval are gates on merging this plan, not Go work.

### Stage 1 acceptance

- `cmp` of stdout, stderr, and exit code is silent for every fixture × check matrix.
- Intent/plan canonical digests match Python for the frozen vectors.
- `internal/evaluate` has no network or git.
- Production workflows still invoke Python.

### Stage 2 acceptance

- Replay matrix has zero Python/Go divergence on findings, order, messages, stdout, stderr, exit codes, and plan/diff digests.
- Trust tests prove base-owned code load and non-execution of head code.
- External-git byte hashes match `sha256sum` of the specified `git diff` stream.
- Benchmark JSON is committed with raw numbers; unexplained regressions block the stage 2 merge, but explained regressions do not retain Python.

### Stage 3 acceptance

- Production path runs one Go binary from immutable base.
- Deletion inventory is empty on `main`.
- No `uv`, `jsonschema`, `pyyaml`, or `validate.py`.
- Machine contexts and rendered labels unchanged.
- Rollback drill documented as `git revert` of the cutover squash.

### Negative cases (must remain failing with the same strings)

Invalid fixtures already on `main`: `invalid-missing-field.json`, `invalid-diff-digest.json`, `invalid-stale-head.json`, `invalid-direct-changelog.json`, `invalid-fragment-deletion.json`, `invalid-fragment-owner.json`, `invalid-fragment-section.json`, `invalid-required-missing.json`. Additional evaluator negatives already covered by `tests/test_validator.py`: duplicate/missing/reordered headings, fenced false authority, legacy-after-v1, broken supersession, non-owner high-risk plan approval, `plan-round`/`implementation-round` limits, shared-identity vs native review, argparse both/neither input, merge-approval blocked by contract, fetched HEAD mismatch, missing `GITHUB_TOKEN`, unsupported content encoding, paginated non-list, and noncanonical record spacing (`repo-ops.merge-review.v1 fields must use one ASCII space`).

Do not add a bypass for Issue #5’s current `## Completion criteria` heading. The diagnostic is `linked issue body is missing ## Acceptance`.

### Explicit non-authority

This plan does not authorize: merging without owner plan approval; starting Go code from this draft; changing rulesets; enabling required checks; tagging or releasing `v0.1.0`; closing Issue #3 or Issue #5; adopting `repo-ops/v0.1.0` in a consumer; keeping Python as a production fallback after stage 3.

If stage 1, 2, or 3 review fails, repair that branch. If compatibility or trust diverges, fix Go until `cmp` is silent. If cutover CI fails, revert the cutover commit. If a defect is found after `v0.1.0` publication (a later Issue #3 gate), do not rewrite that tag.
