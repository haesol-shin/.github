# Migrate repository policy validator to Go

Issue: #5
Current v1 intent: `sha256:1fe86d9dba7114bb75df62a4b31048ba11a9bb9eac4f3bc10fee17882cf71e11`
Supersedes plan: `sha256:0fa489baf002a59d2b43e541670722537fca6125bd84067c68d818497889e888` at plan commit `654d81f68ee1db5baf33c89016d3ddb659e98220`
This file is not implementation authority. Stage 3 work is prohibited until this plan-only pull request merges after owner `repo-ops.plan-approval.v1` naming this file’s exact plan digest and head as `plan-commit`, and exact-head `repo-ops.merge-review.v1`. Merge is a separate user-authorized GitHub action; there is no additional merge-authorization comment schema.

## Approach

Replace the production repository-policy validator with one Go implementation before `repo-ops/v0.1.0` is released. Preserve semantic findings and order, exit codes, canonical intent/plan/diff digests, check conclusions, event behavior, and trust boundaries. Machine contexts remain exactly `quality`, `contract`, and `merge approval`. Python is a temporary compatibility oracle only. After zero semantic divergence, delete the Python validator, uv, and validator runtime dependencies in one clean cutover. Rollback is a normal revert to the last Python production commit, not a second live implementation.

Production at `origin/main` `8afe1f424dac005b2d409043cf899424303669b4` still evaluates policy with base-owned Python. Stage 1 and Stage 2 are merged history. Stage 3 is the remaining work, in two reviewable `Related #5` pull requests with one production validator at every instant:

1. **Preparation** lands the final Go source, collector caps, changelog CLI, and Go behavioral tests. Production action, workflows, and quality command stay Python. After that merge, only the repository owner may build and publish the immutable `linux/amd64` artifact from that trusted `main` commit.
2. **Cutover** is the single clean production cutover. It includes the real pin/provenance record at `actions/repository-policy/artifact.json`, the action rewrite, and Python/uv/harness deletion. The first merged Go production commit is executable immediately. There is no pinless fail-closed window and no long-lived dual production path.

Production policy execution must not compile Go, restore a Go build cache, or fall back to Python.

Compatibility does **not** freeze Python `jsonschema` or PyYAML diagnostic wording, argparse library text, or stdout/stderr whitespace. Do not require rendered workflow/job labels such as `CI / quality` or `Repository policy / contract`. Do not claim those UI strings as machine contexts.

This plan does not rewrite `contracts/v0.1.0`, change policy outcomes, weaken coverage, migrate other repositories, move semantic reasoning into Go, alter repository rulesets, enable enforcement, tag, release, or adopt consumers. Issue #3 remains the release-train authority and waits for this cutover. Issue #5 pull requests use `Related #5` and do not close Issue #5. Implementation pull requests maintain one `changelog.d/5-go-validator.md` fragment. This plan-only pull request declares `Changelog: not-required`.

The merged repository contract is authoritative. Generic Issue/PR heading names in templates and `contract.json` are advisory. Do not reintroduce blocking generic-heading findings or merge-review round counters.

High-risk plan approval is the `repo-ops.plan-approval.v1` pull-request comment binding `issue`, `intent`, `plan`, and `plan-commit`. Exact-head review evidence is `repo-ops.merge-review.v1`. Actual merge is a separate user-authorized GitHub action; no additional merge-authorization comment schema exists, and none was used on PRs #2, #4, #6, #7, #9, #10, #12, or #13. Separate explicit approval remains mandatory for ruleset or required-check changes, tags and GitHub Releases, PyPI, consumer adoption, advisory-to-enforce transition, and platform-smoke failure disposition. Build and publication of the validator artifact are owner-only. If that publication uses a tag or GitHub Release, the existing separate tag/GitHub Release approval boundary applies. Plan approval does not publish anything. No section of this file is any of those approvals.

### Settled history

**Stage 1** merged as PR #10 squash `55e24b49f393cc48a424d24bdf680cfdc7ab0181`. It landed `cmd/repo-ops-validator --fixture`, `internal/{canonical,evaluate,schema,fixture}`, and `tests/testdata/go-oracle/` from the repaired-base Python oracle `654d81f68ee1db5baf33c89016d3ddb659e98220`. Production stayed Python.

**Stage 2** merged as PR #13 squash `8afe1f424dac005b2d409043cf899424303669b4`. It landed `internal/collect`, live `--event`, ten validator replay bundles, three workflow-only intent-revocation bundles, advisory live shadow, and isolated benchmark record `tests/testdata/benchmarks/5-go-validator.json`. Production stayed Python. Exact-head review named head `12fc6b65a7b353eba01fe39a17979a06c01902ce` before squash.

Stage 2 evidence to keep:

- Validator replay: 10 bundles × 3 checks × Python/Go, zero divergence, independently adjudicated `expected.json`.
- Workflow replay: 10 intent-revocation assertions; workflow-only bundles never invoke either validator.
- Advisory live shadow: Python and Go both exited 1 with `no merge-ready receipt was found`; credential value absent from captured bytes.
- Local prebuilt fixture cold start p50: Go 21 ms, Python 258 ms. These are not GitHub `ubuntu-latest` policy-path samples.
- Empty-cache clean-CI p50: Go 26 s, Python 8 s. Go paid toolchain install, module download, and compile-every-run. That compile-every-run cost is why Stage 3 must not build on the live policy path.
- Process-tree RSS p95: Go 9420 KiB, Python 57564 KiB.
- Direct production dependency: `gopkg.in/yaml.v3@v3.0.1`. Toolchain `go 1.22.0` in `go.mod`.
- Residual: collector bounds history depth (`--depth=1`) and the 2-minute live-collection deadline, but not aggregate fetched-object bytes or temp-disk use. No prebuilt-binary supply path or trusted production cache exists.

Do not regenerate Stage 1 goldens, recreate Stage 2 bundles, or re-land the isolated benchmark workflow.

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
| Trust / events | Immutable base, no PR-head execution, same event behavior | Committed replay bundles; one advisory live event |

### Frozen observable contracts

Until the cutover merges, the production entrypoint remains `actions/repository-policy/validate.py`, invoked by `actions/repository-policy/action.yml` through `uv run --project … --locked python … --event "$GITHUB_EVENT_PATH" --check "${{ inputs.check }}"`. CI quality is `.github/workflows/ci.yml` job `quality` (`name: quality`) running `uv run --project actions/repository-policy --locked python -m unittest discover -s tests -v`. Live policy is `.github/workflows/policy.yml` (`pull_request_target`, `pull_request_review`, `issue_comment`) which clones the immutable base SHA, runs the composite action from `.repo-ops/actions/repository-policy`, and publishes check runs named `contract` and `merge approval` on the exact head from one Actions job `publish`. `.github/workflows/repository-policy.yml` is a reusable `workflow_call` entry with the same one-publisher shape; it is not the live central path and must not be called from `policy.yml`. Cutover rewires **both** workflow files plus the composite action to the pinned Go binary.

CLI contract:

- Exactly one of `--fixture` or `--event` is required; both or neither is an error with exit 2. Library argparse wording is not frozen.
- `--check` choices: `all` (default), `contract`, `merge-approval`.
- Exit 0 iff the findings list is empty; otherwise exit 1. Empty findings publish check conclusion `success`; non-empty publish `failure`.
- Caught live failures become a single finding and exit 1. Python `str(error)` / PyYAML wording is not frozen.
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

Live collection to preserve:

- `GITHUB_TOKEN` required for live mode; `GITHUB_API_URL` default `https://api.github.com`.
- User-Agent `repo-ops-validator/0.1`, `X-GitHub-Api-Version: 2022-11-28`, `Authorization: Bearer {token}`, 30s HTTP timeout.
- Pagination follows `Link` `rel="next"`; list endpoints use `per_page=100`; check-runs use `field=check_runs`.
- Policy YAML is loaded from immutable **base** SHA path `.github/repo-policy.yml`.
- Added/modified fragment bytes are loaded from **head** SHA (data only).
- Plan bytes for the current digest are loaded from **head** SHA; plan bytes for an approval commit are loaded from that commit; a plan-commit is accepted only when compare `{plan-commit}...{head}` status is `ahead` or `identical` and both digests equal the approval `plan` field.
- Quality success is the last check-run whose `name` equals `policy.quality.check` (currently `quality`) and whose `conclusion` is `success`. Do not remap that string to a rendered `CI / quality` label.
- Reviews with empty body or `state == DISMISSED` are ignored.
- Trusted code, schemas, pin/provenance document, and the action checkout are the immutable base SHA. Pull-request code is never executed.
- External git: temp dir prefix `repo-ops-`, `git init --quiet`, fetch `refs/pull/{number}/head:refs/repo-ops/head` with `GIT_CONFIG_COUNT=1` `http.https://github.com/.extraheader=AUTHORIZATION: basic <base64(x-access-token:{token})>` when a token is present, require fetched HEAD to equal the GitHub API head SHA, `git cat-file -e {merge_base}^{commit}` and fetch the merge-base if missing, `git update-ref refs/repo-ops/base {merge_base}`, hash diff stdout in 1 MiB reads, fail on non-zero git status. Do not hash stderr. Do not rewrite diff text. Fetches use `--depth=1` and `--no-tags`. Live collection has a 2-minute deadline. Preparation adds aggregate fetched-object byte, temp-disk, and diff-output caps below.

Generic Issue/PR headings are advisory: heading validation returns no findings. Templates and `contract.json` heading lists remain documentation. The `repo-ops.changelog.v1` record, authority lines, risk lines, and fenced machine records remain blocking. `repo-ops.merge-review.v1` field order is `verdict risk intent plan base head diff runtime model` with no `plan-round` or `implementation-round`.

`changelog.py` is not the trusted live validator. CI currently loads it through the same unittest module and `validate.py` imports `parse_fragment` for fragment body checks. Preparation lands `cmd/repo-ops-changelog` while Python folding remains the production oracle. Cutover deletes uv and the validator Python path, so folding is Go-only after that merge. CLI parity is `--root` (default `changelog.d`), `--changelog` (default `CHANGELOG.md`), `--version`, `--date`.

Issue #5 already uses `## Acceptance`. This plan does not edit the Issue. Missing or reordered generic headings are not merge blockers.

Committed replay corpus to keep and to replay with Go after cutover:

- Validator bundles under `tests/testdata/events/validator/<name>/` with `webhook.json`, `http/`, `git/`, and `expected.json` of `{exit, findings, check_conclusion, plan_digest, diff_digest}`: `pull_request_target-opened`, `pull_request_target-edited`, `pull_request_target-reopened`, `pull_request_target-synchronize`, `pull_request_review-submitted`, `pull_request_review-edited`, `pull_request_review-dismissed`, `issue_comment-created-pr`, `issue_comment-edited-pr`, `issue_comment-deleted-pr`.
- Workflow-only bundles under `tests/testdata/events/workflow/<name>/` with workflow expected check-run/revocation outputs: `issue_comment-created-intent`, `issue_comment-edited-intent`, `issue_comment-deleted-intent`. Do not invoke the validator binary on workflow-only bundles.

### Package and command boundaries

One Go module at the repository root, module path `github.com/haesol-shin/.github`, `CGO_ENABLED=0`, toolchain pinned in `go.mod`. After cutover, production policy runners do not install a Go toolchain, do not download modules, and do not compile.

| Path | Responsibility |
| --- | --- |
| `cmd/repo-ops-validator` | CLI parity: `--event`, `--fixture`, `--check` |
| `cmd/repo-ops-changelog` | CLI parity with `changelog.py`: `--root`, `--changelog`, `--version`, `--date` (land in preparation; not production until cutover) |
| `internal/canonical` | `normalize_text`, `canonical_digest`, `plan_digest` |
| `internal/evaluate` | authority lines, records, intent chain, risk authority, changelog state, `validate_state`; headings remain advisory; must not import `net/http` or `os/exec` |
| `internal/schema` | Draft 2020-12 evaluation; failures compared by schema/path/keyword, not Python message text |
| `internal/collect` | `GitHubClient`, `build_live_state`, `api_content`, `git_metadata`, collaborator permissions, resource caps |
| `internal/fixture` | `load_fixture` including `extends` / dotted `replace` |
| `actions/repository-policy/artifact.json` | Single canonical base-owned pin/provenance document; landed in the cutover PR with the real digest; loaded only from the immutable base checkout |
| `tests/testdata/go-oracle/` | Frozen Stage 1 goldens; after cutover compared by Go tests, not a Python runner |
| `tests/testdata/events/` | Frozen Stage 2 replay corpus |

Composite action remains `actions/repository-policy/action.yml`. Do not add a second composite action or a second check name.

Rejected alternatives:

- Dual production Python+Go path after cutover, or any long-lived dual production path.
- Merging a pinless Go production action that fail-closes until a later pin-only change.
- Compiling Go on each policy run from the base checkout or from PR head.
- Using Actions cache, module cache, or build cache as the production validator supply path or as a live fallback.
- Treating SHA-256 equality of downloaded bytes as proof of `source_commit`.
- Embedding CPython, PyOxidizer, or WASM-Python.
- Replacing external git with go-git/git2go (byte stream would drift).
- Renaming machine contexts or collapsing `contract` and `merge approval`.
- Treating rendered labels `CI / quality` or `Repository policy / …` as required contexts.
- Changing evaluator-authored finding text to be “more idiomatic”.
- Freezing `jsonschema` / PyYAML diagnostic strings or stdout/stderr whitespace as cutover gates.
- Reintroducing merge-review round counters or blocking generic-heading findings.
- Treating a live GitHub pull request as the replay oracle.
- New production dependency manager or container bootstrap.
- Publishing the validator artifact as Issue #3 tag `v0.1.0` or any mutable branch/tag.
- Storing the pin/provenance document anywhere other than `actions/repository-policy/artifact.json`.

Dependencies after cutover: the pinned `linux/amd64` binary on the policy path; standard library plus `gopkg.in/yaml.v3@v3.0.1` in the built-from source. `go.mod` / `go.sum` remain for CI quality compilation of PR source. No `jsonschema` / `pyyaml` / uv in production after cutover.

### Production packaging

Stage 3 production path is a trusted-main, owner-authorized, once-built immutable artifact. It is not a compile-every-run path and not a cache-only path.

**Build.** From the exact preparation `origin/main` commit after that commit is on `main`, never from a pull-request head:

```text
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o repo-ops-validator ./cmd/repo-ops-validator
```

That is the only production architecture. The binary is built once per source commit that is authorized to run in production. Rebuild is a new owner-authorized publication, not a policy-job side effect. The cutover PR must not change production Go packages (`cmd/repo-ops-validator`, `cmd/repo-ops-changelog`, `internal/**`). If those packages must change after the artifact exists, restart from a new preparation merge and a new build.

**Publication.** Only the repository owner builds and publishes the exact bytes. Overwriting a previously published digest is forbidden. The store must not be Issue #3 tag `v0.1.0`, a moving `v*` tag, a branch, or a cache key. If publication uses a tag or GitHub Release, the existing separate tag/GitHub Release approval boundary applies. Plan approval does not publish anything.

**Provenance.** SHA-256 of downloaded bytes proves only that those bytes equal the pinned bytes. It does not prove they were produced from `source_commit`. Before the owner records the pin, a trusted main-only procedure must:

1. Build from the exact preparation `source_commit` with the recipe above and the `go` toolchain that satisfies `go.mod` at that commit.
2. Independently rebuild from the same commit, toolchain, `go.mod`/`go.sum` bytes, and recipe, and require identical `sha256` and `size`.
3. Record a durable pin/provenance document binding all of the following. The action later verifies the digest **named by this record**, not a digest offered without provenance.

**Pin/provenance schema.** The single canonical location is the base-owned JSON object `actions/repository-policy/artifact.json`, loaded only from the immutable base checkout, never from `pull_request.head`. Required fields, no others:

| Field | Value |
| --- | --- |
| `source_commit` | 40 lowercase hex of the trusted `main` commit whose Go sources were built |
| `toolchain` | Exact `go version` string used for both builds |
| `goos` | `linux` |
| `goarch` | `amd64` |
| `cgo_enabled` | `"0"` |
| `go_mod_sha256` | `sha256:` plus 64 lowercase hex of `go.mod` bytes at `source_commit` |
| `go_sum_sha256` | `sha256:` plus 64 lowercase hex of `go.sum` bytes at `source_commit` |
| `recipe` | Exact build command string above |
| `sha256` | `sha256:` plus 64 lowercase hex of the `linux/amd64` binary bytes |
| `size` | Decimal integer byte length of those binary bytes |
| `uri` | Immutable URL of those exact bytes |

Unknown fields, missing fields, wrong types, uppercase hex, missing `sha256:` prefix, non-40 `source_commit`, non-positive `size`, or any `goos`/`goarch`/`cgo_enabled` other than the table fail closed. The owner records this document only after the two builds match. A later source change that is allowed to run in production requires a new once-built artifact, a new matching rebuild, and a new pin.

**Digest-before-exec.** The composite action, running from the immutable base checkout, must:

1. Read the pin/provenance document from that base tree.
2. Stream-download the object at `uri` into a runner-temp path that is not the repository workspace. Abort on timeout (30s, the existing HTTP timeout, unless the owner names another) or if received bytes would exceed `size`. Do not write more than `size` bytes.
3. Require received length equals `size`. Compute SHA-256 over the received bytes and require equality with the provenance record’s `sha256`.
4. Hash `go.mod` and `go.sum` in that same immutable base checkout and require equality with `go_mod_sha256` and `go_sum_sha256`. Refuse to exec unless `goos`/`goarch`/`cgo_enabled` match the table and `source_commit` is 40 lowercase hex. Do not substitute a branch name, tag, or `git describe`. Do not treat binary digest equality as source provenance.
5. Set `REPO_OPS_CONTRACT_ROOT` to the immutable base checkout root that contains `contracts/v0.1.0/contract.json`, derived from `github.action_path` (the composite action directory’s repository root, two parents up), never from the caller workspace, never from PR head, never from the temp executable directory.
6. Exec the verified file with `--event "$GITHUB_EVENT_PATH" --check "${{ inputs.check }}"` and that environment.

Verification, including length and digest, runs on every policy execution, including warm runs. Missing pin, unparseable pin, download failure, timeout, length mismatch, digest mismatch, truncated or oversized body, or architecture mismatch is a finding and exit 1. The action must not exec an unverified file, must not compile, and must not fall back to cache or Python.

**Authority and update lifecycle.**

- **Create.** Repository owner only, after the preparation source commit is on `origin/main`. The owner runs the two matching builds and publishes the bytes. There is no additional artifact-create comment schema. If publication uses a tag or GitHub Release, that existing approval boundary applies. The first production pin is committed as `actions/repository-policy/artifact.json` in the cutover PR, so that PR’s merged tree is executable.
- **Update.** Repository owner only. A later source change that is allowed to run in production requires a new once-built artifact, a new matching rebuild, and a new pin. Pin updates are not inferred from CI green.
- **Unavailable.** Fail closed as above. There is no compile fallback, cache fallback, or Python fallback.
- **No pinless production merge.** Do not merge an `action.yml` that requires the pin until `actions/repository-policy/artifact.json` in the same tree contains the real digest, size, and URI.

**PR-source CI versus production policy.**

- `.github/workflows/ci.yml` job `quality` after cutover checks out the reviewed source and runs `go test ./...`. It may install Go from `go.mod` and may use a CI module/build cache. That compilation is a quality check of PR source. It never publishes `contract` or `merge approval`, never supplies the production binary, and never writes the production pin. Until cutover, `quality` stays the Python unittest command.
- `.github/workflows/policy.yml` and `.github/workflows/repository-policy.yml` never run `go build`, never run `setup-go`, never restore `GOCACHE`/`GOMODCACHE` for the validator, and never execute a binary from PR head or from an unverified download.

### Cache-only is not the production path

Cache-only is rejected as both the production path and a live fallback.

Trust-domain isolation fails: GitHub Actions caches are not an owner-authorized, digest-pinned artifact. Cache keys can be populated by jobs that see untrusted pull-request writes or by a prior toolchain/OS/arch mix. Production policy must run bytes the owner published, not bytes the last writer of a cache key happened to store.

Cache-miss compile cost is the Stage 2 measurement: empty-cache Go CI p50 26 s versus Python 8 s because the job installed a toolchain, downloaded modules, and compiled every sample. A production policy job that compiles on miss repeats that cost on the trusted path and makes latency a function of cache luck.

Provenance is “whatever last filled this key”, not an owner-authorized `CGO_ENABLED=0 linux/amd64` build of `source_commit` with matching rebuild, `sha256`, and `size`. A digest-before-exec pin can be attached to a cache object only by turning the cache into an ad-hoc artifact store without immutability. That is the publication design without the authority boundary.

Therefore: do not restore a Go build/module cache in `policy.yml` or `repository-policy.yml`; do not `go build` when the pin is missing; do not treat cache-hit as verification.

### Collector resource bounds

Preparation must close the Stage 2 residual in the binary that will be published. Live `GitMetadata` and live collection fail closed when any cap is exceeded. Exceeding a cap is a single finding and exit 1. Caps apply to the live collector, not to rewriting diff bytes.

Required caps:

- **Aggregate fetched-object bytes:** total git pack/object bytes fetched in one `GitMetadata` call, including the pull-head fetch and any merge-base fetch. Depth flags are not a substitute.
- **Temp-disk:** peak size of the `repo-ops-` temporary directory, including objects, pack files, and diff spill.
- **Diff-output bytes:** counted while hashing the specified `git diff` stdout; stop and fail closed before unbounded buffering.
- **Time:** keep the Stage 2 2-minute live-collection deadline and 30s HTTP timeout; git subprocesses inherit the deadline.

This plan does not invent numeric byte/disk caps. Stage 2 evidence records no fetched-object or temp-dir sizes. The owner must name the three numeric caps from measured replay-bundle and representative live-PR maxima plus documented headroom. Tune only from that evidence; do not pick round numbers without measurements. Record the measurements and the chosen caps in the preparation pull request.

## Execution

1. Merge this plan-only pull request after owner `repo-ops.plan-approval.v1` on the unchanged 40-hex head naming this file’s plan digest and that head as `plan-commit`, and exact-head `repo-ops.merge-review.v1`. Then merge it as a separate user-authorized GitHub action. Do not require a further merge-authorization comment. Do not start Stage 3 implementation before that merge. Do not treat this draft as merge, release, ruleset, artifact-publication, or cutover authority.

2. The preparation pull request records the measured replay/live maxima and selected aggregate fetched-object, temp-disk, and diff-output caps in its evidence comment, with documented headroom. Exact-head implementation review evaluates those choices before its merge-ready receipt. Artifact download timeout defaults to the existing 30s HTTP timeout unless the reviewed implementation records evidence for a different value.

   The single pin/provenance location is `actions/repository-policy/artifact.json`. The repository owner chooses the build vehicle, immutable store, and URI at publication time within this plan’s constraints; those operational values are captured in the provenance document rather than a new approval record.

3. **Preparation pull request** against then-current `main`, `Related #5`, maintaining `changelog.d/5-go-validator.md`. High-risk plan approval on that pull request names this plan digest and this plan commit. Production stays Python. In that pull request:

   - Land collector caps with fail-closed tests.
   - Land `cmd/repo-ops-changelog` with `changelog.py` CLI parity and move fragment parsing used by evaluation into Go.
   - Land Go tests that cover the fixture matrix, digest vectors, schema-failure identity, validator event replay (inject a non-compiling binary or in-process evaluator; do not treat `go run` of PR head as production), collector trust tests, changelog folding, and cap failures.
   - Do not change `action.yml`, `policy.yml`, `repository-policy.yml`, `ci.yml` quality command, or `repo-policy.yml` `quality.commands`.
   - Do not add a dual production path, pin, or `setup-go` on policy jobs.

   After exact-head `repo-ops.merge-review.v1` on the unchanged preparation head, merge as a separate user-authorized GitHub action. Do not require a further merge-authorization comment. Production remains Python.

4. **Owner-only build and publish.** After preparation is on `origin/main`, only the repository owner builds from that exact SHA (owner-local or a main-only owner procedure; never `pull_request`), independently rebuilds with the same vehicle class, recipe, and toolchain, requires identical `sha256` and `size`, and publishes the immutable artifact to a digest-immutable store that is not `v0.1.0` or a mutable ref. Plan approval does not publish anything. If publication uses a tag or GitHub Release, the existing separate tag/GitHub Release approval boundary applies. Do not record the pin on `main` yet.

5. **Python policy baseline** on GitHub `ubuntu-latest`, n ≥ 5, while Python is still production, before cutover. Stage 2 local 21 ms and empty-cache 26 s are not this baseline. Use the production policy surface:

   - Job: `.github/workflows/policy.yml` job `publish` (or an isolated measurement that invokes the same composite action with the same event payload, check order, runner OS, and permissions).
   - Invocations: `--check contract` then `--check merge-approval`, matching production.
   - Timing boundary: wall time of each composite-action invocation, recorded separately; also record job duration.
   - RSS: process-tree VmRSS of the validator invocation on the runner; name the method in the record.
   - Cold: first invocation in the job; no prior validator artifact on the runner.
   - Warm: second same-job invocation; record cache state (what was reused; no Go compile cache exists on this path).
   - Store every sample, then p50/p95.

6. **Advisory live zero-divergence** while Python is still production: one live event, Python production vs the published Go binary, same `--event`/`--check`, zero semantic divergence, credential value absent from captured bytes. Go-only replay of committed fixtures and validator bundles must already be silent from the preparation tests.

7. **Cutover pull request**, the single clean production cutover, `Related #5`, maintaining `changelog.d/5-go-validator.md`. High-risk plan approval names this plan digest and this plan commit. It must include the real pin/provenance document for the already-published artifact. In that pull request:

   - Add `actions/repository-policy/artifact.json` with the real provenance fields, digest, size, and URI. No dummy digest.
   - Change `actions/repository-policy/action.yml` to the digest-before-exec procedure, including bounded streaming download, `REPO_OPS_CONTRACT_ROOT` from `github.action_path`, and the existing `--event` / `--check` flags. Do not install uv. Do not compile Go.
   - Change `.github/workflows/ci.yml` job `quality` to `go test ./...` with Go installed from `go.mod`. That job compiles PR source for tests only.
   - Keep `.github/workflows/policy.yml` and `.github/workflows/repository-policy.yml` on the one-publisher shape, immutable-base checkout, and check names `contract` and `merge approval`. They consume the rewired composite action and must not gain `setup-go` or `go build`.
   - Change `.github/repo-policy.yml` `quality.commands` to `go test ./...` while keeping `quality.check: quality`.
   - Update `changelog.d/README.md` folding command to `cmd/repo-ops-changelog` with the same `--version` / `--date` flags. Update every remaining in-repo operator document that instructs `uv run`, `validate.py`, or `python actions/repository-policy/changelog.py` for this validator. Do not rewrite historical `.ops/plans/` for other Issues.
   - Fold the cutover into `changelog.d/5-go-validator.md`. Do not edit `CHANGELOG.md`; Issue #3 folding remains the release train.
   - Delete `.github/workflows/5-go-validator-benchmark.yml`.
   - Delete the Python inventory and convert or delete every Python-dependent harness listed below. Frozen JSON, goldens, and event bundles stay.
   - Do not modify production Go packages. Do not add a dual path, compatibility shim, feature flag, or fallback.
   - Trust tests must exercise the **exact** composite-action invocation: pin from a base-like tree, download to temp, `REPO_OPS_CONTRACT_ROOT` from the action-path analog, `--event` and `--check`. `go run` from the workspace is not that test.

   After exact-head `repo-ops.merge-review.v1` on the unchanged cutover head, merge as a separate user-authorized GitHub action. Do not require a further merge-authorization comment. The merged tree is immediately executable.

8. **Deletion inventory** (empty on `main` after cutover):

   - `actions/repository-policy/validate.py`
   - `actions/repository-policy/changelog.py`
   - `actions/repository-policy/pyproject.toml`
   - `actions/repository-policy/uv.lock`
   - `actions/repository-policy/.python-version`
   - `astral-sh/setup-uv@37802adc94f370d6bfd71619e3f0bf239e1f3b78` from `action.yml` and `ci.yml`
   - `uv run --project … python …` production and CI steps
   - Python-only unittest runner from CI
   - `tests/test_validator.py` after Go behavioral coverage
   - `tests/testdata/go-oracle/compare.py` (delete or replace with a Go-only reader; do not leave a Python executable that imports the removed validator or `jsonschema`)
   - `tests/testdata/events/replay.py` (delete or replace with a Go-only reader that injects a non-compiling binary; do not leave a default `python,go` runner)
   - `tests/testdata/benchmarks/collect_local.py` (delete or replace; do not leave a runner that requires uv or `validate.py`)
   - `.github/workflows/5-go-validator-benchmark.yml`

   Do not delete `contracts/`, `fixtures/` JSON states, `.github/repo-policy.yml`, check publication logic, committed `tests/testdata/go-oracle/` goldens, `tests/testdata/events/validator/*/expected.json`, or `tests/testdata/events/workflow/*/expected.json`.

9. **Post-cutover Go policy samples** on the same GitHub `ubuntu-latest` surface, n ≥ 5, same event, check order, timing boundary, and RSS method as the Python baseline. Cold is the first composite-action invocation in the job with no prior artifact on the runner (download, length check, digest, exec). Warm is the second same-job invocation that still verifies length and digest before exec; record cache state (verified file reused or HTTP cache of the immutable blob; never a compile cache). Compare Go p50/p95, peak RSS p95, policy-path step count, and production dependency count to the Python policy baseline, not to Stage 2 local or empty-cache CI figures. Any worse Go value requires an `explanations[]` entry. Measurements are evidence only and never retain Python. Verify post-merge `quality` on the exact cutover `main` SHA.

10. **Compatibility, security, and exact-head review** required on each implementation head before merge:

    - Preparation: Go tests silent against committed `{exit, findings, check_conclusion, plan_digest, diff_digest}` for every fixture and validator event bundle; workflow-only bundles asserted by workflow expected outputs; cap and folding tests pass; production still Python.
    - Cutover: the same Go-only replay; trust tests prove pin, schemas, action, policy YAML, and `REPO_OPS_CONTRACT_ROOT` come from base; head modifications of `actions/repository-policy/**` or `contracts/**` do not change findings relative to base-owned copies; PR-head Python/Go is not executed; workflow permissions remain `contents: read`, `issues: read`, `pull-requests: read`, `checks: write`; the binary does not request scopes.
    - Artifact failure paths in tests: missing pin, unparseable pin, unknown field, wrong `goos`/`goarch`/`cgo_enabled`, unreachable `uri`, timeout, empty body, truncated body, oversized body, length mismatch, SHA-256 mismatch, `go.mod`/`go.sum` digest mismatch against the base checkout, missing `REPO_OPS_CONTRACT_ROOT`, `REPO_OPS_CONTRACT_ROOT` pointing at caller workspace or PR head, and attempt to exec before length and digest equality. Each fails closed with no fallback.
    - External-git byte hashes still match `sha256sum` of the specified `git diff` stream, including binary file, `core.quotePath` path, deletion, and empty diff.

11. **Rollback.** Revert the cutover squash to restore the last Python production commit (the preparation `main` SHA, or a later Python-production SHA if `main` moved). Revert of preparation is a normal revert and does not change production wiring. Do not land a feature flag or parallel action. Do not keep the Go pin as a live fallback after revert.

## Verification and recovery

### This plan-only pull request

The diff contains this plan plus removal of `TestPlanDigestRealPlanBytes`, which pinned the previous plan’s source bytes and made every legitimate plan revision fail `go test ./...`. Canonical digest behavior remains covered by fixed known-answer vectors; mutable plan prose is authority through the approved plan digest and commit, not a compiled test constant. No implementation code, fixtures, changelog fragments, workflows, harness, pin, or contract edits are allowed.

- `quality` and the advisory Go benchmark jobs — pass; neither treats mutable plan source text as a compiled constant.
- `contract` — fail closed until owner plan approval names this file’s exact digest and plan commit.
- `merge approval` — fail closed unless contract succeeded and an exact-head `repo-ops.merge-review.v1` exists.

Do not interpret a failed contract on this draft as permission to change the contract or to start Stage 3. Merge this plan-only pull request after owner `repo-ops.plan-approval.v1` and exact-head `repo-ops.merge-review.v1`; then merge it as a separate user-authorized GitHub action. Do not require a further merge-authorization comment. The current committed plan digest `sha256:0fa489baf002a59d2b43e541670722537fca6125bd84067c68d818497889e888` is superseded by this file; implementation requires that merge of the **new** exact digest and plan commit.

### Stage 1 and Stage 2

Already merged at the SHAs above. Do not re-gate them. Durable contracts and replay artifacts remain binding on Stage 3.

### Preparation acceptance

- Production path is still Python in `action.yml`, `ci.yml`, `policy.yml`, `repository-policy.yml`, and `repo-policy.yml` `quality.commands`.
- Collector caps and `cmd/repo-ops-changelog` are on `main`.
- Go tests cover the observable contracts listed above without changing production wiring.
- Owner `repo-ops.plan-approval.v1` and exact-head `repo-ops.merge-review.v1` existed for the merged preparation head; merge was a separate user-authorized GitHub action.
- The repository owner built and published the artifact from that `main` SHA after two matching trusted-main builds; `actions/repository-policy/artifact.json` is not yet required on `main`.
- Python policy baseline n ≥ 5 is recorded on the production policy surface.
- Advisory live Python-vs-Go event showed zero semantic divergence.

### Cutover acceptance

- Production policy path runs one owner-built `linux/amd64` binary from the immutable base after provenance-named SHA-256 and size verification in `action.yml`, `policy.yml`, and `repository-policy.yml`.
- The merged cutover tree contains the real pin at `actions/repository-policy/artifact.json` and is executable without a follow-up pin commit.
- `REPO_OPS_CONTRACT_ROOT` is set from `github.action_path` to the immutable base checkout; trust tests cover that exact invocation.
- CI quality is `go test ./...`; `quality.check` remains `quality`.
- No policy-path `go build`, `setup-go`, Go cache restore, uv, `jsonschema`, `pyyaml`, `validate.py`, `compare.py`, `replay.py`, or `collect_local.py`.
- Pin/provenance schema matches the table; length and digest are verified before every exec; unavailable artifact fails closed.
- Collector aggregate fetched-object, temp-disk, and diff-output caps are enforced and fail closed; time limits remain.
- Deletion inventory is empty on `main`. Frozen goldens and event expected files remain.
- `cmd/repo-ops-changelog` is the folding command; `changelog.d/README.md` matches it.
- Machine contexts remain `quality`, `contract`, and `merge approval`.
- Go-only binary replay matches committed validator `expected.json` for every fixture and validator event bundle. Workflow-only bundles are not binary-replayed.
- Post-cutover Go policy cold/warm samples are compared to the pre-cutover Python policy baseline on the same surface.
- Security review and exact-head `repo-ops.merge-review.v1` existed for the merged cutover head; merge was a separate user-authorized GitHub action.
- `Related #5` still does not close Issue #5.
- Rollback is `git revert` of the cutover squash to the last Python production commit.
- No `v0.1.0` tag, `v*` protection, GitHub Release for the contract, ruleset change, Issue #3 close, or consumer adoption.

### Negative cases (must remain failing with the same evaluator findings or structured schema identity)

Invalid fixtures already on `main`: `invalid-missing-field.json`, `invalid-diff-digest.json`, `invalid-stale-head.json`, `invalid-direct-changelog.json`, `invalid-fragment-deletion.json`, `invalid-fragment-owner.json`, `invalid-fragment-section.json`, `invalid-required-missing.json`. Additional evaluator negatives already covered: fenced false authority, legacy-after-v1, broken supersession, non-owner high-risk plan approval, shared-identity vs native review, argparse both/neither input (exit 2), merge-approval blocked by contract, fetched HEAD mismatch, missing `GITHUB_TOKEN`, unsupported content encoding, paginated non-list, and noncanonical record spacing. Schema required-field omissions must still fail with structured identity; the Python `jsonschema` sentence is not the identity.

Must **not** fail: missing, duplicate, or reordered generic Issue/PR headings (advisory). Must **not** require `plan-round` / `implementation-round` on merge-review receipts. Reintroducing those round fields must fail schema/field-order tests. `quality.check: CI / quality` must fail schema validation.

### Explicit non-authority

This plan does not authorize: merging this plan-only pull request without owner `repo-ops.plan-approval.v1` of this file’s exact digest and head and exact-head `repo-ops.merge-review.v1`; starting Stage 3 implementation before that merge; compiling Go on the production policy path; using cache or Python as a fallback; merging a pinless Go production action; storing the pin anywhere other than `actions/repository-policy/artifact.json`; changing rulesets; enabling required checks; tagging or releasing `v0.1.0`; closing Issue #3 or Issue #5; adopting `repo-ops/v0.1.0` in a consumer; publishing the validator artifact except as an owner-only build of trusted `main` after two matching rebuilds; treating plan approval as publication.

If preparation or cutover review fails, repair that branch. If compatibility or trust diverges, fix Go until the harness artifacts match. If cutover CI fails, revert the cutover commit. If a defect is found after `v0.1.0` publication (a later Issue #3 gate), do not rewrite that tag.
