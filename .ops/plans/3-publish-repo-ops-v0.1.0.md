# Publish repository operations contract v0.1.0

Issue: #3
Intent: `sha256:ee7be0d0eea38584c8bd54db5c6cd5046596d2fe4b03d07243799ada5a7638e0`

## Approach

Complete the unadopted `repo-ops/v0.1.0` bundle before its first consumer freezes the contract. The release adds the changelog lifecycle that the operating standard already requires, changes schema identifiers to dereferenceable raw-tag URLs, and establishes protected immutable release tags. It does not adopt a consumer, enable consumer enforcement, change receipt or risk authority, deploy, or publish automatically.

The pull request `Impact` section gains one exact declaration:

```text
- Changelog: required — <why users or operators need a release note>
- Changelog: not-required — <why no release note is needed>
- Changelog: release — vX.Y.Z
```

`required` requires at least one added or modified fragment under the policy-declared root. `not-required` requires a non-empty rationale but no fragment. `release` is reserved for a release pull request, permits the generated changelog update and fragment consumption, and names the version being prepared. Ordinary pull requests may not edit `CHANGELOG.md` or delete fragments.

The repository policy uses `changelog.mode: fragments` with `root: changelog.d`. Issue-backed fragments use `<issue>-<slug>.md`, and the numeric prefix must equal the linked issue. Permitted direct low-risk changes use `direct-<slug>.md`; that form is invalid on an issue-backed route. A fragment contains one or more allowed Keep a Changelog sections and at least one non-empty bullet under each section. `changelog.d/README.md` documents the format and is not a fragment.

A deterministic repository-owned command validates fragments and folds them in stable filename order into `CHANGELOG.md` for the version and date supplied by the release preparer. Folding rejects an existing version, malformed or empty fragments, and a dirty or incomplete fragment set; it updates the changelog and removes only the consumed fragments. The trusted policy validator checks the PR declaration and changed-file lifecycle without executing pull request code.

All schema `$id` values change from GitHub HTML blob URLs to `https://raw.githubusercontent.com/haesol-shin/.github/v0.1.0/...` so a resolver receives JSON after publication. Reusable workflows still require a full commit SHA; the tag is the human-facing schema and release identity, not a workflow trust input.

The current plan file is part of the feature candidate pull request and its approved commit remains in that pull request's history. That candidate declares `Changelog: required`, adds `changelog.d/3-repo-ops-v0.1.0.md`, and does not create or edit `CHANGELOG.md`. After it merges, the folding command prepares a separate release pull request that declares `Changelog: release — v0.1.0`, creates the initial `CHANGELOG.md`, and deletes the consumed fragment. The release pull request references the same approved plan and receives its own plan-approval and exact-head review records.

Feature merge, release-pull-request merge, tag protection, release approval, annotated tag creation, and GitHub Release publication remain separate gates. Before publication, the maintainer must approve the repository ruleset change protecting `v*` and later approve the exact verified release-pull-request commit. After publication, never move, delete, or recreate `v0.1.0`; correct defects with a new version or document a blocking limitation.

## Execution

1. Commit this plan, open the draft feature pull request linked to Issue #3, complete plan review, and record the maintainer's high-risk plan approval for the final plan digest and plan commit.
2. Extend `contract.json`, the pull request template, and conformance fixtures with the exact `Changelog` declaration and allowed values.
3. Extend the trusted validator's live state with paginated pull request file metadata and enforce declaration presence, rationale, fragment requirement, issue/direct ownership, filename and section syntax, direct `CHANGELOG.md` protection, and release-only fragment deletion.
4. Add deterministic fragment validation and release folding as repository-owned code with focused tests for ordering, malformed fragments, duplicate versions, and safe consumption.
5. Add `changelog.d/README.md`, `changelog.d/3-repo-ops-v0.1.0.md`, and an accurate repository policy using fragment mode with release enabled; the feature pull request declares `Changelog: required` and does not touch `CHANGELOG.md`.
6. Change all three schema `$id` values to raw `v0.1.0` URLs and update alignment tests without changing the schema payload contracts.
7. Run the complete central conformance suite, exercise fragment validation and folding on a disposable copy, verify patch integrity, and obtain an adversarial exact-head review.
8. Squash-merge the feature candidate with a concise bullet body after maintainer merge approval, then verify the exact `main` commit and post-merge CI.
9. From that verified `main`, run the folding command to prepare a separate release pull request declaring `Changelog: release — v0.1.0`; it creates the dated `CHANGELOG.md` entry and deletes only the consumed fragment.
10. Validate the release pull request under the new base-owned contract, record plan approval for the unchanged plan naming the squashed `main` commit from step 8 as `plan-commit`, obtain exact-head review, merge after maintainer approval, and verify the exact release candidate and post-merge CI.
11. Request explicit approval for the `v*` tag protection ruleset, apply and verify it, then request separate release approval naming the exact release candidate.
12. After release approval, create annotated tag `v0.1.0` and publish GitHub Release notes with `Added`, `Changed`, `Fixed`, `Compatibility`, `Upgrade`, `Known limitations`, and `Rollback` sections. The notes identify advisory mode as non-blocking, shared-identity review as not independently authenticated, and the full commit SHA consumers must pin.
13. Verify the tag target, release URL, raw schema URLs, release-note sections, and consumer pin.

## Verification and recovery

Before the feature merge, run `python -m unittest discover -s tests -v` and `git diff --check`. The conformance suite must prove all three changelog declarations, required-fragment presence, rationale enforcement, issue-number ownership, direct-route ownership, allowed sections, non-empty bullets, stable folding order, duplicate-version rejection, ordinary changelog-edit rejection, release-only fragment deletion, and unchanged intent, plan, review, risk, and heading behavior.

Run the folding command against a disposable copy containing representative fragments. Verify the exact generated `CHANGELOG.md`, consumed file set, stable ordering, and refusal to overwrite an existing version; discard only this rehearsal output. Confirm repository policy schema validation, raw schema `$id` alignment, and that no consumer workflow accepts a branch or tag in place of the full commit SHA.

After the feature merge, require green `CI / quality`, then use the shipped folding command to create the real release pull request. Its checks must prove that the generated `CHANGELOG.md` matches the fragment content, the correct fragment is deleted, no unrelated file is consumed, and the `release` declaration names `v0.1.0`. Require green post-merge CI on the exact release candidate.

Before publication, verify the `v*` ruleset blocks tag update and deletion, compare the proposed tag target with the release candidate, and record the separate release approval. After publication, verify the annotated tag resolves to the approved commit, the GitHub Release targets the same tag, every required release-note section is present, and every raw `v0.1.0` schema URL returns the expected JSON.

If planning, feature review, release review, or CI fails, repair the applicable branch without tagging. If release-candidate verification or tag protection fails, revert the release candidate and do not publish. If a defect is found after publication, preserve the immutable tag, state the limitation in the release, block consumer adoption when necessary, and publish a new corrective version rather than rewriting history.
