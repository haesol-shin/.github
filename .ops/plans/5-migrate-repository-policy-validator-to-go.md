# Migrate repository policy validator to Go

Issue: #5
Current v1 intent: `sha256:ab01092464d6f25e1385604c238a0fb4a9eee37aa3c58c912cb616f07e21651c`
Supersedes: `sha256:3c5898cf216583590419b74147ed57bf4af27a762d3ee5a470d93036a6750cb7`
Frozen production oracle: `fe229702e88cda5e1fb7ad142112edb50fd57c82` (unconditional; `origin/main` after PR #9)

## Approach

Replace the production repository-policy validator with one Go implementation before `repo-ops/v0.1.0` is released. Preserve semantic findings (codes/meaning) and order, exit codes, canonical intent/plan/diff digests, check conclusions, event behavior, and trust boundaries. Machine contexts remain exactly `quality`, `contract`, and `merge approval`. Python is a temporary compatibility oracle only. After zero semantic divergence, delete the Python validator, uv, and validator runtime dependencies in one clean cutover. Rollback is a normal revert to the last Python production commit, not a second live implementation.

The oracle SHA `fe229702e88cda5e1fb7ad142112edb50fd57c82` is frozen by this plan. Replacing it requires editing this file, a new plan digest, and owner re-approval. Do not silently rebase the oracle onto later `main`.

Compatibility does **not** freeze Python `jsonschema` or PyYAML diagnostic wording, argparse library text, or stdout/stderr whitespace. Do not require rendered workflow/job labels such as `CI / quality` or `Repository policy / contract`. Do not claim those UI strings as machine contexts.

This plan does not rewrite `contracts/v0.1.0`, change policy outcomes, weaken coverage, migrate other repositories, move semantic reasoning into Go, alter repository rulesets, enable enforcement, tag, release, or adopt consumers. Issue #3 remains the release-train authority and waits for this cutover. Issue #5 pull requests use `Related #5`. Implementation pull requests after this plan-only artifact maintain one `changelog.d/5-go-validator.md` fragment. This plan-only pull request declares `Changelog: not-required`.

PR #7 is this unmerged plan-only pull request. Policy smoke is the **repaired merged-base** publisher (PR #9 on `fe229702e88cda5e1fb7ad142112edb50fd57c82`) evaluating and publishing onto the **unmerged PR #7 head**. `pull_request_target` does not run workflows from that head. Smoke must pass, then owner plan approval must land, then an exact-head evidence comment on that same 40-hex head must record the smoke outputs, before Stage 1 starts.

The merged repository contract is authoritative. Generic Issue/PR heading names in templates and `contract.json` are advisory. Do not reintroduce blocking generic-heading findings or merge-review round counters during the port.

### Compatibility by finding class

| Class | Frozen identity | Comparison |
| --- | --- | --- |
| Evaluator-authored findings | Exact strings and order (`validate_state` / `validate_changelog_state` text, including `contract check did not succeed; merge approval is blocked` at merge-approval index 0) | Ordered `::warning title=Repository policy::` suffixes |
| Schema failures | Structured identity and order: schema id/label, instance path, keyword/code (required/type/const/…). Same instances fail. | Not Python `jsonschema` `error.message` |
| YAML policy load | Structured object identity for `.github/repo-policy.yml`; load failures are one finding, exit 1, stable order relative to other findings | Not PyYAML exception text |
| Presentation | Annotation prefix plus ordered finding payloads. Exit 0 iff findings empty (`success`); else exit 1 (`failure`). | Human summary lines, step-summary Markdown, and stdout/stderr whitespace are not frozen |
| CLI usage | Both/neither `--fixture`/`--event` → exit 2 | argparse wording is not frozen |
| Canonical intent/plan digests | Exact `sha256:` hex from the algorithm below | Explicit digest-vector files, not inferred from stdout |
| Diff digest | Exact SHA-256 of the specified external `git diff` stdout byte stream | `sha256sum` of that stream |
| Secrets | Exact non-disclosure of the **injected secret value** | Scan captured bytes for that value, not the substring `GITHUB_TOKEN` |
| Check conclusions | Empty findings → `success`; non-empty → `failure` | Derived from exit/findings |
| Trust / events | Immutable base, no PR-head execution, same event behavior | Replay bundles below |

### Frozen current behavior

The production entrypoint is `actions/repository-policy/validate.py`, invoked by `actions/repository-policy/action.yml` through `uv run --project … --locked python … --event "$GITHUB_EVENT_PATH" --check "${{ inputs.check }}"`. CI quality is `.github/workflows/ci.yml` job `quality` (`name: quality`) running `uv run --project actions/repository-policy --locked python -m unittest discover -s tests -v`. Live policy is `.github/workflows/policy.yml` (`pull_request_target`, `pull_request_review`, `issue_comment`) which clones the immutable base SHA, runs the composite action from `.repo-ops/actions/repository-policy`, and publishes check runs named `contract` and `merge approval` on the exact head from one Actions job `publish`. `.github/workflows/repository-policy.yml` is a reusable `workflow_call` entry with the same one-publisher shape; it is not the live central path and must not be called from `policy.yml`. Stage 3 wires **both** workflow files plus the composite action.

CLI contract to preserve:

- Exactly one of `--fixture` or `--event` is required; both or neither is an error with exit 2. Library argparse wording is not frozen.
- `--check` choices: `all` (default), `contract`, `merge-approval`.
- Exit 0 iff the findings list is empty; otherwise exit 1. Empty findings publish check conclusion `success`; non-empty publish `failure`.
- Caught live failures (`OSError`, `RuntimeError`, `ValueError`, YAML parse errors) become a single finding and exit 1. Python `str(error)` / PyYAML wording is not frozen.
- For `--check merge-approval`, evaluate contract first; if contract findings exist, insert `contract check did not succeed; merge approval is blocked` at index 0 of the merge-approval findings, then emit merge-approval findings.
- Presentation: each evaluator finding is the suffix of `::warning title=Repository policy::{finding}`, in evaluator order.

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
| `internal/schema` | Draft 2020-12 evaluation; failures compared by schema/path/keyword, not Python message text |
| `internal/collect` | `GitHubClient`, `build_live_state`, `api_content`, `git_metadata`, collaborator permissions |
| `internal/fixture` | `load_fixture` including `extends` / dotted `replace` |
| `tests/testdata/go-oracle/compare.py` | Stage 1 checked-in differential harness (contract below; not part of this plan-only diff) |

Composite action remains `actions/repository-policy/action.yml`. Production wiring changes only in stage 3: build the validator from the immutable base checkout and exec the binary with the same `--event` / `--check` flags in `action.yml`, `.github/workflows/ci.yml`, `.github/workflows/policy.yml`, and `.github/workflows/repository-policy.yml`. Do not add a second composite action or a second check name.

Rejected alternatives:

- Dual production Python+Go path after cutover.
- Embedding CPython, PyOxidizer, or WASM-Python.
- Replacing external git with go-git/git2go (byte stream would drift).
- Renaming machine contexts or collapsing `contract` and `merge approval`.
- Treating rendered labels `CI / quality` or `Repository policy / …` as required contexts.
- Changing evaluator-authored finding text to be “more idiomatic”.
- Freezing `jsonschema` / PyYAML diagnostic strings or stdout/stderr whitespace as cutover gates.
- Reintroducing merge-review round counters or blocking generic-heading findings.
- Silently rebasing the frozen oracle SHA.
- Treating a live GitHub pull request as the Stage 2 replay oracle.
- New production dependency manager or container bootstrap.

Dependencies: standard library for HTTP, git subprocess, JSON, SHA-256, and CLI. YAML parser only if it yields the same policy object as the frozen `.github/repo-policy.yml` data model (today: scalars, nested maps, one list). Prefer `gopkg.in/yaml.v3` only after a fixture proves equivalent `policy` objects. Vendor or `go.mod` sums are required before cutover. No `jsonschema`/`pyyaml`/uv in production after cutover.

### Three reviewable implementation stages

All stages are separate `Related #5` pull requests against the then-current `main`. High-risk plan approval on each implementation pull request names this plan digest and the plan commit that introduced it (or a later reviewed plan commit if this file changes). There is no plan-round or implementation-round limit.

**Stage 1 — freeze and fixture evaluator.** Export language-independent semantic golden artifacts from the Python oracle **at** `fe229702e88cda5e1fb7ad142112edb50fd57c82` only. Land `cmd/repo-ops-validator --fixture` plus `internal/{canonical,evaluate,schema,fixture}` with no GitHub or git collection. Land the checked-in harness. Production workflows stay Python. Changelog fragment `changelog.d/5-go-validator.md` is created here (`Changelog: required`). Do not start this stage until PR #7 smoke, owner plan approval, and the exact-head evidence comment exist.

**Stage 2 — collector, replay, shadow, benchmarks.** Implement `internal/collect` and live `--event`. Land deterministic **validator** replay bundles (webhook + recorded HTTP + git objects) and run both implementations against the same local API/git. Land separate **workflow-only** intent-revocation bundles with check-run/revocation expected outputs; those are not CLI matrix cases. A real pull request is advisory shadow only. Record benchmarks, including Go clean-CI from an isolated non-production procedure. Production workflows still call Python. Repair every semantic divergence; do not cut over.

**Stage 3 — clean cutover.** Point `action.yml`, `ci.yml`, `policy.yml`, and `repository-policy.yml` at the Go binary built from the immutable base. Delete the Python validator path, uv, runtime dependencies, and the Stage 2 benchmark-only workflow. Port folding to `cmd/repo-ops-changelog`. Update `repo-policy.yml` `quality.commands` to the Go test invocation while keeping `quality.check: quality`. Replay committed **validator** event expected outputs with Go only. No dual path, no extra bootstrap.

## Execution

1. Merge this plan-only pull request only after (a) owner `repo-ops.plan-approval.v1` naming this file’s plan digest and plan commit, (b) PR #7 policy smoke of the repaired merged base against this unmerged head has passed, and (c) an exact-head evidence comment on that same 40-hex head records the smoke recipe outputs. Do not start Go code before all three. Do not treat this draft as approval, merge, release, or ruleset authority.

2. **PR #7 policy smoke** (gates Stage 1). Vehicle: unmerged PR #7 head `$HEAD` (40-hex). Publisher: repaired merged base `fe229702e88cda5e1fb7ad142112edb50fd57c82`, not this head’s tree. Working directory may be any clone with `gh` authenticated read. Create an evidence directory, then run the whole recipe (do not stop after check-runs):

```text
mkdir -p .ops/evidence/5-go-validator/pr7-smoke
HEAD="<40-hex unmerged PR #7 head>"

gh api --paginate "repos/haesol-shin/.github/commits/${HEAD}/check-runs?per_page=100" \
  > .ops/evidence/5-go-validator/pr7-smoke/check-runs.json

gh api --paginate "repos/haesol-shin/.github/actions/runs?head_sha=${HEAD}&per_page=100" \
  > .ops/evidence/5-go-validator/pr7-smoke/workflow-runs.json

jq -r '.workflow_runs[]?.id // empty' .ops/evidence/5-go-validator/pr7-smoke/workflow-runs.json \
  > .ops/evidence/5-go-validator/pr7-smoke/run-ids.txt

: > .ops/evidence/5-go-validator/pr7-smoke/publish-job-id
while IFS= read -r RUN_ID; do
  [ -n "$RUN_ID" ] || continue
  gh api --paginate "repos/haesol-shin/.github/actions/runs/${RUN_ID}/jobs?per_page=100" \
    > ".ops/evidence/5-go-validator/pr7-smoke/jobs-${RUN_ID}.json"
  jq -r '.jobs[] | [.id, .name, (.conclusion // "")] | @tsv' \
    ".ops/evidence/5-go-validator/pr7-smoke/jobs-${RUN_ID}.json"
  jq -r '.jobs[] | select(.name=="publish") | .id' \
    ".ops/evidence/5-go-validator/pr7-smoke/jobs-${RUN_ID}.json" \
    >> .ops/evidence/5-go-validator/pr7-smoke/publish-job-id
done < .ops/evidence/5-go-validator/pr7-smoke/run-ids.txt

PUBLISH_JOB="$(head -n 1 .ops/evidence/5-go-validator/pr7-smoke/publish-job-id)"
gh api "repos/haesol-shin/.github/actions/jobs/${PUBLISH_JOB}/logs" \
  > .ops/evidence/5-go-validator/pr7-smoke/publish.log
```

   Accept all of:

   - Latest check-run `quality` is `success` when CI passed.
   - Latest check-runs `contract` and `merge approval` exist as custom results on `$HEAD`.
   - No Actions **job** `name` equals `contract` or `merge approval`.
   - An Actions job `name` equals `publish`.
   - No check-run `name` equals `CI / quality`.
   - `publish.log` contains a successful `uses: ./.repo-ops/actions/repository-policy` load (no YAML/`action.yml` parse error).
   - Revoke-then-final may yield multiple check-run rows per policy context; that is not duplication of Actions jobs.

   If smoke fails, do not start Go; repair against `main` or revert the PR #9 squash. Do not change rulesets to compensate.

   After owner `repo-ops.plan-approval.v1` on this head, post a PR comment on the **same** 40-hex head that quotes `check-runs.json` summaries, every Actions job name from `jobs-*.json`, and the `publish.log` excerpt proving composite load. Stage 1 must not start without that exact-head evidence comment.

3. **Stage 1 freeze.** From repository root, using the Python oracle **only** at `fe229702e88cda5e1fb7ad142112edb50fd57c82`, write golden files under `tests/testdata/go-oracle/` (gitignored working copy under `.ops/evidence/5-go-validator/oracle/` is acceptable during authoring):

   - For every `fixtures/*.json` except `changelog-conformance.json` and `intent-comment-revocation.json`, run `--check all`, `--check contract`, and `--check merge-approval`.
   - Capture `{exit, findings, check_conclusion}` where `findings` is every `::warning title=Repository policy::` suffix in order, and `check_conclusion` is `success` iff `exit == 0` else `failure`. Do not treat full stdout/stderr bytes as goldens.
   - Commit explicit digest vectors `tests/testdata/go-oracle/digests.json` from `tests/test_validator.py` intent and plan cases (exact `sha256:` bytes). The harness compares both implementations to this file.
   - Capture schema-failure identity by feeding each frozen schema an empty object and each required-field omission: record schema label, instance path, and keyword/code, not Python `error.message`.
   Commit goldens with the Go evaluator and the harness. `go test ./internal/... ./cmd/repo-ops-validator` must pass without network.

4. **Stage 1 differential harness contract** (landed in Stage 1 as `tests/testdata/go-oracle/compare.py`; this plan-only PR does not add it). The harness, not an inline snippet, is the comparison authority:

   - Create the output directory (`mkdir -p`); do not assume it exists.
   - Run Python then Go for each fixture × check; capture stdout, stderr, and exit into separate files. Expected nonzero exits must not abort the matrix (`exit` is written after each process; the harness continues).
   - Parse ordered finding payloads from `::warning title=Repository policy::` suffixes.
   - Compare `{exit, findings, check_conclusion}` per the compatibility table.
   - Compare canonical intent/plan (and, when present, diff) digests to `tests/testdata/go-oracle/digests.json`, not by scraping stdout.
   - Inject a distinct secret value into the child environment (the token string actually passed). Fail if that **value** appears in stdout, stderr, or step summary. Do not treat the literal substring `GITHUB_TOKEN` as the leak oracle.
   - Do not `cmp` stdout, stderr, or step-summary files.

   `$FIXTURE` is every file in `fixtures/*.json` that `load_fixture` accepts as a validator state (`changelog-conformance.json` is folding-only; `intent-comment-revocation.json` is workflow-only). `$CHECK` is `all`, `contract`, and `merge-approval`. Working directory is the repository root. `GITHUB_STEP_SUMMARY` is unset during comparison so summary presentation cannot mask finding payloads.

5. **Stage 1 Go package rules.** `internal/evaluate` must not import `net/http` or `os/exec`. `internal/collect` must not be referenced from fixture tests. Evaluator-authored finding order equals Python append order in `validate_state` / `validate_changelog_state`.

6. **Stage 2 collector.** Implement live `--event` with the frozen HTTP, pagination, base-owned policy load, head-owned fragment/plan load, permission lookup, and external-git hashing. Unit-test collectors with recorded HTTP fixtures, not the live API.

7. **Stage 2 deterministic replay bundles.** Split two classes. Do not mix them.

   **Validator bundles** live under `tests/testdata/events/validator/<name>/`:

   | Member | Role |
   | --- | --- |
   | `webhook.json` | Sanitized `policy.yml` delivery (`github.event`); tokens/authorization/extraheader values replaced by `REDACTED` |
   | `http/` | Recorded GitHub API responses (method+path+query), including pagination `Link` bodies |
   | `git/` | Git objects or a bundle sufficient to reproduce `refs/repo-ops/base` and `refs/repo-ops/head` |
   | `expected.json` | `{exit, findings, check_conclusion, plan_digest, diff_digest}` only — committed for Python/Go CLI comparison and Go-only post-cutover binary replay |

   Both Python and Go **must** consume the same local HTTP and local git from that bundle. Do not call live GitHub or a real working tree during the validator CLI matrix. A real pull request shadow run is advisory evidence only, not the replay oracle.

   Required **validator** bundle names (minimum set; add rather than drop):

   | Directory | `github.event_name` | `action` | Purpose |
   | --- | --- | --- | --- |
   | `pull_request_target-opened` | `pull_request_target` | `opened` | first contract+merge-approval publication |
   | `pull_request_target-edited` | `pull_request_target` | `edited` | body/title authority change |
   | `pull_request_target-reopened` | `pull_request_target` | `reopened` | re-open |
   | `pull_request_target-synchronize` | `pull_request_target` | `synchronize` | exact-head change |
   | `pull_request_review-submitted` | `pull_request_review` | `submitted` | merge-review/plan-approval on review |
   | `pull_request_review-edited` | `pull_request_review` | `edited` | stale review body |
   | `pull_request_review-dismissed` | `pull_request_review` | `dismissed` | dismissed reviews ignored by collector |
   | `issue_comment-created-pr` | `issue_comment` | `created` | PR comment with plan-approval or merge-review |
   | `issue_comment-edited-pr` | `issue_comment` | `edited` | approval edit revocation |
   | `issue_comment-deleted-pr` | `issue_comment` | `deleted` | approval deletion revocation |

   The Stage 1 harness (or a Stage 2 extension of it) runs both implementations against **validator** bundles only and compares `{exit, findings, check_conclusion, plan_digest, diff_digest}` to `expected.json` and to each other. Secret scan uses the injected bundle token **value**.

   **Workflow-only intent-revocation bundles** live under `tests/testdata/events/workflow/<name>/`. They cover Issue (non-PR) `issue_comment` deliveries that `policy.yml` job `intent-revocation` handles. They are **not** validator CLI inputs.

   | Member | Role |
   | --- | --- |
   | `webhook.json` | Sanitized Issue comment delivery (`created` / `edited` / `deleted`) |
   | `http/` | Recorded list of linked open pull heads and Checks API revoke POSTs |
   | `expected.json` | Workflow-level `{job, linked_open_pull_heads, check_runs: [{name, conclusion, head_sha}]}` — not `{exit, findings, …}` |

   Required **workflow-only** bundle names (minimum set; add rather than drop):

   | Directory | `github.event_name` | `action` | Purpose |
   | --- | --- | --- | --- |
   | `issue_comment-created-intent` | `issue_comment` | `created` | Issue intent comment; revoke linked open PR heads |
   | `issue_comment-edited-intent` | `issue_comment` | `edited` | intent supersession; revoke |
   | `issue_comment-deleted-intent` | `issue_comment` | `deleted` | intent deletion; revoke |

   Workflow expected `check_runs` are `contract` and `merge approval` with `conclusion: failure` on each linked open pull `head_sha`. Replay these by asserting `intent-revocation` job control flow and recorded Checks API outputs. **Exclude** them from the Python/Go CLI matrix and from Go-only binary replay. Do not invoke `cmd/repo-ops-validator` or `validate.py` on workflow-only bundles.

8. **Stage 2 immutable-base trust checks** (must fail closed in tests):

- Validator binary, `action.yml`, schemas, and `.github/repo-policy.yml` used by production come from the base SHA checkout under `.repo-ops`, never from `pull_request.head`.
- A fixture where head modifies `actions/repository-policy/**` or `contracts/**` does not change findings relative to base-owned copies.
- The injected secret **value** is not present in stdout, stderr, or step summary.
- Workflow permissions remain `contents: read`, `issues: read`, `pull-requests: read`, `checks: write` on policy jobs; the Go binary does not request scopes.
- Executing PR-head Python/Go is a failed trust test, not a feature.

9. **Stage 2 external-git byte hashing evidence.** For a known merge-base/head pair inside a replay bundle’s `git/`:

```text
git -c core.quotePath=true diff --binary --full-index --no-color --no-ext-diff --no-textconv --no-renames "$BASE" "$HEAD" -- | sha256sum
```

The validator `diff_digest` must equal `sha256:` plus that hex. Include a binary file, a path needing `core.quotePath`, a deletion, and an empty diff. Go-git hashes are not evidence.

10. **Stage 2 benchmark evidence** (record, do not gate cutover). Production `quality` stays Python until Stage 3, so it cannot produce Go clean-CI samples. Collect Go clean-CI from an **isolated benchmark-only** workflow on the Stage 2 branch, not from production `ci.yml` / `policy.yml`.

   Land `.github/workflows/5-go-validator-benchmark.yml` on the Stage 2 branch only:

   - Job name is not `quality`, `contract`, or `merge approval`.
   - Does not publish Checks API results and does not change production wiring.
   - Runs the **exact** proposed Stage 3 quality command (`go test ./...`) from the Stage 2 tree.
   - Empty cache each sample (no restored Go/module/action caches).
   - n ≥ 5 comparable samples (matrix or repeated `workflow_dispatch`).
   - Delete this workflow in the Stage 3 cutover PR; it is not production.

   Write `tests/testdata/benchmarks/5-go-validator.json` with per-language raw samples and summaries:

```json
{
  "oracle_commit": "fe229702e88cda5e1fb7ad142112edb50fd57c82",
  "go_commit": "<40-hex>",
  "local_cold_start_ms": {
    "python": {"samples": [], "p50": 0, "p95": 0, "n": 0},
    "go": {"samples": [], "p50": 0, "p95": 0, "n": 0}
  },
  "clean_ci_wall_ms": {
    "python": {"samples": [], "p50": 0, "p95": 0, "n": 0, "source": "production quality job"},
    "go": {"samples": [], "p50": 0, "p95": 0, "n": 0, "source": "5-go-validator-benchmark.yml"}
  },
  "peak_rss_kib": {
    "python": {"samples": [], "p50": 0, "p95": 0, "n": 0},
    "go": {"samples": [], "p50": 0, "p95": 0, "n": 0}
  },
  "workflow_steps": {"python": 0, "go": 0},
  "production_dependencies": {"python": ["jsonschema==4.25.1", "pyyaml==6.0.2", "uv"], "go": ["<module>@<version>", "..."]},
  "explanations": []
}
```

Commands: local cold start is the wall time of a fresh process `--fixture fixtures/valid-high.json --check all` after dropping OS file cache when practical; n ≥ 5; store every sample then p50/p95. Peak RSS is max `ru_maxrss` / equivalent per run, with the same sample/summary shape. Python clean-CI wall is the production `quality` job duration on an empty-cache workflow_run. Go clean-CI wall is the isolated benchmark workflow duration for `go test ./...` on an empty cache, n ≥ 5; it is not the production `quality` job. Workflow step count for Go is the **proposed** production `policy.yml` step count after cutover (counted from the Stage 2 tree’s intended wiring), not the benchmark-only job. Dependency count is production trusted-path modules only.

Any worse Go **p95**, **peak RSS p95**, **workflow step count**, or **production dependency count** versus Python is a regression and requires an `explanations[]` entry. Benchmarks are evidence only. They never authorize retaining Python.

11. **Stage 3 cutover.** In one pull request: build `repo-ops-validator` from the base checkout in `action.yml`, `policy.yml`, **and** `repository-policy.yml`; replace CI `uv run … unittest` with `go test ./...`; change `quality.commands` to that Go test line; wire `cmd/repo-ops-changelog` for release folding; delete the Python inventory below and `.github/workflows/5-go-validator-benchmark.yml`; keep check publication names `contract` and `merge approval`. After merge, verify post-merge `quality` on the exact `main` SHA.

12. **Deletion inventory (stage 3, complete cutover):**

- `actions/repository-policy/validate.py`
- `actions/repository-policy/changelog.py`
- `actions/repository-policy/pyproject.toml`
- `actions/repository-policy/uv.lock`
- `actions/repository-policy/.python-version`
- `astral-sh/setup-uv@37802adc94f370d6bfd71619e3f0bf239e1f3b78` from `action.yml`, `ci.yml`, `policy.yml`, and `repository-policy.yml`
- `uv run --project … python …` production steps
- Python-only unittest runner from CI (tests themselves move to `go test`)
- `.github/workflows/5-go-validator-benchmark.yml`

Do not delete `contracts/`, `fixtures/` JSON states, `.github/repo-policy.yml`, check publication logic, committed `tests/testdata/events/validator/*/expected.json`, or `tests/testdata/events/workflow/*/expected.json`. `tests/test_validator.py` is deleted only when Go tests cover the same observable contracts.

13. **Post-cutover verification.** Replay every frozen fixture and every **validator** event bundle with the Go binary only against committed `{exit, findings, check_conclusion, plan_digest, diff_digest}`. Do not run the binary against workflow-only intent-revocation bundles; confirm those with workflow expected check-run/revocation outputs. Confirm check-run names remain `quality`, `contract`, and `merge approval`. Confirm `Related #5` still does not close Issue #5. Do not tag `v0.1.0`, protect `v*`, publish a GitHub Release, change rulesets, or close Issue #3.

14. **Rollback.** Revert the stage 3 squash commit (or the contiguous cutover commits) to restore the last Python production commit. Do not land a feature flag or parallel action. Stages 1–2 rollback is likewise revert; Python remains production until stage 3 merges.

## Verification and recovery

### Plan-only pull request (this change)

The diff must contain only `.ops/plans/5-migrate-repository-policy-validator-to-go.md`. No Go code, fixtures, changelog fragments, workflows, tests, harness, or contract edits. Policy smoke is the repaired merged-base publisher on this unmerged head. Expected live checks:

- `quality` — pass if the plan file does not affect unittest discovery.
- `contract` — fail closed until owner plan approval exists.
- `merge approval` — fail closed unless contract succeeded and an exact-head `repo-ops.merge-review.v1` exists.

Those publications are the proof that the repaired base-owned contexts exist. Do not interpret a failed contract on this draft as permission to change the contract or skip later stages. Owner plan approval, passing smoke recipe, and the exact-head evidence comment are gates on merging this plan and on starting Go, not Go work itself.

### Stage 1 acceptance

- Checked-in harness comparison is silent for every fixture × check matrix: matching exit codes, ordered evaluator findings, structured schema/YAML identities, and check conclusions.
- Intent/plan canonical digests match `tests/testdata/go-oracle/digests.json` (exact digest bytes).
- Harness creates its output directory, continues after expected nonzero exits, and scans the injected secret value.
- `internal/evaluate` has no network or git.
- Production workflows still invoke Python.

### Stage 2 acceptance

- Validator event bundles include webhook, recorded HTTP, git objects, and `{exit, findings, check_conclusion, plan_digest, diff_digest}`.
- Workflow-only intent-revocation bundles include webhook, recorded HTTP, and check-run/revocation expected outputs; they are absent from the CLI matrix.
- Validator replay matrix has zero Python/Go divergence against those expected artifacts using the same local API/git. Live PR shadow is advisory only.
- Trust tests prove base-owned code load and non-execution of head code.
- External-git byte hashes match `sha256sum` of the specified `git diff` stream.
- Injected secret values never appear in stdout, stderr, or step summary.
- Go clean-CI samples come from the isolated benchmark-only workflow (n ≥ 5, empty cache, exact proposed `go test ./...`), not production `quality`.
- Benchmark JSON includes per-language samples and summaries; unexplained worse Go p95/RSS/steps/dependency count blocks the stage 2 merge; explained regressions still do not retain Python.

### Stage 3 acceptance

- Production path runs one Go binary from immutable base in `action.yml`, `ci.yml`, `policy.yml`, and `repository-policy.yml`.
- Deletion inventory is empty on `main`.
- No `uv`, `jsonschema`, `pyyaml`, or `validate.py`.
- Machine contexts remain `quality`, `contract`, and `merge approval`.
- Go-only binary replay matches committed validator `expected.json` for every fixture and validator event bundle. Workflow-only bundles are not binary-replayed.
- Rollback drill documented as `git revert` of the cutover squash.

### Negative cases (must remain failing with the same evaluator findings or structured schema identity)

Invalid fixtures already on `main`: `invalid-missing-field.json`, `invalid-diff-digest.json`, `invalid-stale-head.json`, `invalid-direct-changelog.json`, `invalid-fragment-deletion.json`, `invalid-fragment-owner.json`, `invalid-fragment-section.json`, `invalid-required-missing.json`. Additional evaluator negatives already covered by `tests/test_validator.py`: fenced false authority, legacy-after-v1, broken supersession, non-owner high-risk plan approval, shared-identity vs native review, argparse both/neither input (exit 2), merge-approval blocked by contract, fetched HEAD mismatch, missing `GITHUB_TOKEN`, unsupported content encoding, paginated non-list, and noncanonical record spacing. Schema required-field omissions must still fail with structured identity; the Python `jsonschema` sentence is not the identity.

Must **not** fail: missing, duplicate, or reordered generic Issue/PR headings (advisory). Must **not** require `plan-round` / `implementation-round` on merge-review receipts. Reintroducing those round fields must fail schema/field-order tests. `quality.check: CI / quality` must fail schema validation.

### Explicit non-authority

This plan does not authorize: merging without owner plan approval; starting Go before PR #7 smoke and the exact-head evidence comment; changing rulesets; enabling required checks; tagging or releasing `v0.1.0`; closing Issue #3 or Issue #5; adopting `repo-ops/v0.1.0` in a consumer; keeping Python as a production fallback after stage 3; replacing the frozen oracle SHA without a new plan digest.

If stage 1, 2, or 3 review fails, repair that branch. If compatibility or trust diverges, fix Go until the harness artifacts match. If cutover CI fails, revert the cutover commit. If a defect is found after `v0.1.0` publication (a later Issue #3 gate), do not rewrite that tag.
