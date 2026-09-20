# Publish repository operations contract v0.1.0

Issue: #3
Intent: `sha256:3a1c474bf4d01a00de459986226dd58d6b2e2844741df8dad02bdab3b089c934`

## Approach

Complete the unadopted `repo-ops/v0.1.0` bundle before its first consumer freezes the contract. The release adds explicit chained intent authority, non-closing Issue links, the changelog lifecycle required by the operating standard, dereferenceable raw-tag schema identifiers, and protected immutable release tags. It does not adopt a consumer, enable consumer enforcement, change plan or review authority, deploy, or publish automatically.

Design discussion remains in the mutable Issue body. After scope stabilizes, the maintainer records one accepted intent comment:

````markdown
**Intent accepted**

<Resolved outcome, invariants, and non-goals.>

- Risk: `high`
- Intent digest: `sha256:<digest>`
- Supersedes: `none | sha256:<previous-digest>`

Authoritative machine-readable record:

```text
repo-ops.intent.v1 decision:accepted risk:high intent:sha256:<digest> supersedes:<none|sha256:previous-digest>
```
````

The human display omits redundant Issue and Decision fields. The trusted validator derives the Issue number from the GitHub comment surface and computes the intent digest from canonical `{issue, outcome, risk}`. The first v1 record uses `supersedes:none`; a replacement must name the current digest. Displayed values must match the fenced record. Intermediate design comments are not intent records.

The bootstrap preserves audit continuity from the legacy accepted-intent format. Before the bootstrap pull request, one final legacy record is computed from the same stabilized outcome and current `{outcome, risk}` algorithm. After the new validator reaches `main`, the first v1 record uses the issue-bound digest and names the legacy digest in `supersedes`. Later replacements form one validated chain.

An issue-backed pull request uses exactly one standalone authority line. `Fixes #N` and `Closes #N` link accepted intent and ask GitHub to close the Issue on merge; `Related #N` links the same authority without closing it. Multiple or conflicting authority lines are invalid. Every v0.1.0 pull request uses `Related #3`, and the coordinating Issue closes only after the tag and GitHub Release are verified.

The bootstrap pull request must land before the new authority forms can govern later work. Because its immutable base understands neither `Related #N` nor `repo-ops.intent.v1`, it carries one maintainer-approved §12 bootstrap exception naming the exact head, the two expected contract findings, its expiry at merge, and compensating plan, CI, and adversarial review evidence. It changes only intent authority and non-closing Issue linkage. The subsequent feature and release pull requests are fully governed by the new base-owned contract.

The pull request `Impact` section gains one exact changelog declaration:

```text
- Changelog: required — <why users or operators need a release note>
- Changelog: not-required — <why no release note is needed>
- Changelog: release — vX.Y.Z
```

`required` requires at least one added or modified fragment under the policy-declared root. `not-required` requires a non-empty rationale but no fragment. `release` is reserved for a release pull request, permits the generated changelog update and fragment consumption, and names the version being prepared. Ordinary pull requests may not edit `CHANGELOG.md` or delete fragments.

The repository policy uses `changelog.mode: fragments` with `root: changelog.d`. Issue-backed fragments use `<issue>-<slug>.md`, and the numeric prefix must equal the linked issue. Permitted direct low-risk changes use `direct-<slug>.md`; that form is invalid on an issue-backed route. A fragment contains one or more allowed Keep a Changelog sections and at least one non-empty bullet under each section. `changelog.d/README.md` documents the format and is not a fragment.

A deterministic repository-owned command validates fragments and folds them in stable filename order into `CHANGELOG.md` for the version and date supplied by the release preparer. Folding rejects an existing version, malformed or empty fragments, and a dirty or incomplete fragment set; it updates the changelog and removes only consumed fragments. The trusted validator checks PR declarations and changed-file lifecycle without executing pull request code.

All schema `$id` values change from GitHub HTML blob URLs to `https://raw.githubusercontent.com/haesol-shin/.github/v0.1.0/...` so resolvers receive JSON after publication. Reusable workflows still require a full commit SHA; the tag is the human-facing schema and release identity, not a workflow trust input.

The feature candidate uses `Related #3`, declares `Changelog: required`, adds `changelog.d/3-repo-ops-v0.1.0.md`, and does not edit `CHANGELOG.md`. After it merges, the folding command prepares a separate release pull request that also uses `Related #3`, declares `Changelog: release — v0.1.0`, creates the initial `CHANGELOG.md`, and deletes the consumed fragment.

Bootstrap merge, feature merge, release merge, tag protection, release approval, annotated tag creation, GitHub Release publication, and Issue closure remain separate gates. Before publication, the maintainer must approve the ruleset change protecting `v*` and later approve the exact verified release candidate. After publication, never move, delete, or recreate `v0.1.0`; correct defects with a new version or document a blocking limitation.

## Execution

1. Stabilize this plan, compute the legacy and issue-bound v1 digests from the same final outcome, record one final legacy accepted-intent comment, and obtain maintainer approval of the reviewed plan.
2. Prepare a narrow bootstrap pull request containing this plan, `repo-ops.intent.v1` schema and contract declaration, legacy-to-v1 migration, supersession validation, the readable fenced comment format, `Related #N` parsing, authority-line cardinality checks, pull request template guidance for `Fixes`, `Closes`, `Related`, and direct low-risk routes, and focused fixtures.
3. Record the §12 bootstrap exception for the exact bootstrap head, verify the expected base-policy findings and compensating evidence, obtain adversarial exact-head review, and merge only after maintainer approval.
4. After bootstrap merge and green post-merge CI, post the first v1 intent record on Issue #3 with the issue-bound digest and `supersedes` set to the final legacy digest.
5. Prepare the feature pull request with `Related #3`, record plan approval for the unchanged plan naming the squashed bootstrap commit from step 3 as `plan-commit`, and extend `contract.json`, the pull request template, and fixtures with the exact changelog declaration and values.
6. Extend the trusted validator's live state with paginated pull request file metadata and enforce changelog declaration presence, rationale, fragment requirement, issue/direct ownership, filename and section syntax, direct `CHANGELOG.md` protection, and release-only fragment deletion.
7. Add deterministic fragment validation and release folding as repository-owned code with focused tests for ordering, malformed fragments, duplicate versions, and safe consumption.
8. Add `changelog.d/README.md`, `changelog.d/3-repo-ops-v0.1.0.md`, and an accurate repository policy using fragment mode with release enabled; do not touch `CHANGELOG.md`.
9. Change all schema `$id` values to raw `v0.1.0` URLs and update alignment tests without changing schema payload contracts.
10. Complete conformance and realistic folding verification, obtain exact-head review and maintainer merge approval, squash-merge the feature candidate, and verify the exact `main` commit and post-merge CI.
11. From verified `main`, run the folding command to prepare a release pull request using `Related #3` and `Changelog: release — v0.1.0`; create the dated `CHANGELOG.md` entry and delete only the consumed fragment.
12. Validate the release pull request under the new base-owned contract, record plan approval naming the squashed feature commit as `plan-commit`, obtain exact-head review, merge after maintainer approval, and verify the exact release candidate and post-merge CI.
13. Request explicit approval for the `v*` tag-protection ruleset, apply and verify it, then request separate release approval naming the exact release candidate.
14. After release approval, create annotated tag `v0.1.0` and publish GitHub Release notes with `Added`, `Changed`, `Fixed`, `Compatibility`, `Upgrade`, `Known limitations`, and `Rollback` sections. Identify advisory mode as non-blocking, shared-identity review as not independently authenticated, and the full commit SHA consumers must pin.
15. Verify the tag target, release URL, raw schema URLs, release-note sections, and consumer pin, then close Issue #3 as completed.

## Verification and recovery

The bootstrap conformance suite must prove exact intent comment formatting, displayed-to-machine value agreement, issue-bound canonical digests, first-record and replacement supersession, rejection of broken chains, legacy-to-v1 migration, `Related #N` authority, preserved `Fixes` and `Closes` behavior, and rejection of multiple or conflicting authority lines. Its exact-head review must confirm the implementation changes no plan, review, risk, release, or deployment authority.

The bootstrap pull request is expected to expose only the two findings caused by its immutable base not recognizing `Related #3` or v1 intent. The exception record must name those findings, the exact head and diff digest, compensating green `CI / quality` and review evidence, expiry at bootstrap merge, and the requirement that every later pull request pass the new contract without exception.

Before the feature merge, run `python -m unittest discover -s tests -v` and `git diff --check`. The suite must prove all changelog declarations, required-fragment presence, rationale enforcement, issue-number and direct-route ownership, allowed sections, non-empty bullets, stable folding order, duplicate-version rejection, ordinary changelog-edit rejection, release-only fragment deletion, and unchanged authority behavior.

Run folding against a disposable copy containing representative fragments. Verify the exact generated `CHANGELOG.md`, consumed file set, stable ordering, and refusal to overwrite an existing version; discard only rehearsal output. Confirm repository policy schema validation, raw schema `$id` alignment, and that no consumer workflow accepts a branch or tag instead of a full commit SHA.

After the feature merge, require green `CI / quality`, then use the shipped folding command for the real release pull request. Prove that the generated changelog matches fragment content, the correct fragment is deleted, no unrelated file is consumed, and the release declaration names `v0.1.0`. Require green post-merge CI on the exact release candidate.

Before publication, verify the `v*` ruleset blocks tag update and deletion, compare the proposed tag target with the release candidate, and record separate release approval. After publication, verify the annotated tag resolves to the approved commit, the GitHub Release targets the same tag, every required release-note section is present, and every raw schema URL returns expected JSON.

If bootstrap review or its exception is rejected, do not merge it or start feature work. If planning, feature review, release review, or CI fails, repair the applicable branch without tagging. If release-candidate verification or tag protection fails, revert the release candidate and do not publish. After publication, preserve the immutable tag, state any defect as a limitation, block consumer adoption when necessary, and publish a corrective version rather than rewriting history.
