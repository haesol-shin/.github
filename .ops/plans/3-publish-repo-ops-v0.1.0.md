# Publish repository operations contract v0.1.0

Issue: #3
Current v1 intent: `sha256:3a1c474bf4d01a00de459986226dd58d6b2e2844741df8dad02bdab3b089c934`

## Current checkpoint

The contract, schemas, changelog lifecycle, reusable workflow, and Go validator are merged. Issue #5 is complete. Production now uses the immutable checksum-pinned Go artifact introduced by PR #18, and post-merge quality passed on exact main commit `6f310d48eccc86493ee89961a31f9fae1a178024`. No `v0.1.0` tag or GitHub Release exists, no `v*` protection has been approved, and no consumer has adopted the contract.

This revision is understood as completing the immutable repo-ops release and its required non-merged LecturAL proof. Canonical authority tooling, workflow latency optimization, and LecturAL production adoption remain separately authorized work after publication. notice-bot and agent-skills adoption are outside the approved scope.

## Canonical inputs

- Release authority and accepted intent: Issue #3.
- Risk, approval, record, and exact-head rules: `contracts/v0.1.0/contract.json` and its referenced schemas.
- Repository settings: `.github/repo-policy.yml`.
- Consumer entrypoint: `.github/workflows/repository-policy.yml`.
- Release folding implementation: `cmd/repo-ops-changelog` and `internal/changelog`.
- Release fragments: every filename matching the repository's fragment grammar directly under `changelog.d`, excluding `README.md`; enumerate the exact set from the candidate base immediately before folding.
- Published bundle inventory: `contracts/v0.1.0/**`, `actions/repository-policy/**`, `.github/workflows/repository-policy.yml`, and `.github/PULL_REQUEST_TEMPLATE.md` at the exact candidate commit.

Execute from a clean checkout of `haesol-shin/.github` whose local `main` equals remote `main`. GitHub comments and reviews carry plan approval, exact-head merge review, owner merge approval, ruleset approval, and exact-candidate release approval; a plan file or passing check never supplies those approvals by itself. The contract defines who may author machine approvals. Ruleset and release approvals must be owner comments on Issue #3 that name the proposed configuration or exact candidate SHA.

## Approach

Prepare one exact release candidate from verified main. The release pull request uses `Related #3` and `repo-ops.changelog.v1 kind:release value:v0.1.0`, runs the Go folding command, creates the initial `CHANGELOG.md`, and removes exactly the complete validated fragment set. It must not change contract semantics, policy authority, workflow topology, rulesets, or consumer repositories.

Before publication, exercise that exact candidate from a non-merged LecturAL pull request. LecturAL keeps its repository-specific Issue and pull-request prose, owns its `quality` workflow, and pins the central reusable workflow by the candidate's full 40-hex commit SHA. The proof must cover immutable validator loading, custom check publication, denial-to-recovery, exact-head invalidation, permissions, and the absence of a stale transport failure. The integration pull request remains unmerged until an immutable repo-ops release exists.

A consumer-visible blocker found by the integration proof is repaired before release. Any candidate change invalidates the proof and requires a new exact candidate and a repeated LecturAL run. Convenience tooling and performance improvements do not block v0.1.0 unless the consumer proof shows they are required for correctness or safe adoption.

Tag protection and release publication are separate authority gates. After the candidate and consumer proof pass, explicitly approve and verify a `v*` ruleset that blocks tag update and deletion. Then separately approve the exact release commit. Create annotated tag `v0.1.0` and its GitHub Release only after both approvals. Never move, delete, or recreate the published tag; defects require a corrective version.

## Invariants

- Issue #3 remains the release authority and closes only after publication verification.
- Release-train pull requests use exactly one standalone `Related #3` line.
- The current accepted intent, risk authority, plan approval, and exact-head merge-review contracts do not change.
- Machine contexts remain `quality`, `contract`, and `merge approval`.
- Shared-identity review is evidence, not independently authenticated human approval.
- Trusted validator code and schemas come from immutable pinned sources; pull-request code is never executed by the policy validator.
- Reusable workflow consumers pin a full commit SHA, never a branch or mutable tag.
- The initial release folds every validated fragment enumerated from the candidate base immediately before folding; `README.md` is never a release fragment.
- This plan does not authorize required-check enforcement, LecturAL production adoption, product deployment, PyPI publication, notice-bot adoption, or agent-skills adoption.

## Execution

1. Merge this plan-only revision after owner plan approval, green `quality` and `contract`, exact-head merge review, and explicit owner merge approval. It changes no runtime behavior and declares `repo-ops.changelog.v1 kind:not-required value:plan-only`.
2. From verified main, rehearse folding in a disposable clean copy, including the duplicate-version, malformed-fragment, dirty-changelog, and incomplete-fragment rejection paths; discard the output.
3. Create a release branch from the unchanged verified main and run `go run ./cmd/repo-ops-changelog --version v0.1.0 --date <UTC release date>`. Compare the command's consumed paths with the Canonical inputs list. Retain only the generated `CHANGELOG.md` and deleted fragments.
4. Open the release pull request with `Related #3`, `Risk: high`, `Plan: .ops/plans/3-publish-repo-ops-v0.1.0.md`, and `repo-ops.changelog.v1 kind:release value:v0.1.0`. Obtain current plan approval, green `quality` and `contract`, an exact-head merge-review record, and explicit owner merge approval. Merge only the reviewed head.
5. Verify post-merge `quality` on the exact main commit. That commit becomes the only release candidate. Record its full SHA and `git ls-tree -r --name-only <candidate>` output for every Published bundle inventory root.
6. In `haesol-shin/LecturAL`, open a dedicated test Issue and a non-merged integration pull request against its default branch. Pin both `uses: haesol-shin/.github/.github/workflows/repository-policy.yml@<candidate>` and `contract-ref: <candidate>` to the same full candidate SHA. Use a thin caller workflow plus LecturAL-owned policy, `quality` workflow, and templates; do not copy central policy shell logic.
7. On one fixed LecturAL head, capture the Actions jobs and check runs for these transitions: an intentionally incomplete authority state produces failed `contract` and `merge approval`; valid Issue intent and pull-request authority recover both expected contexts after current quality succeeds; deleting or editing exact-head authority revokes the prior results; restoring valid authority republishes success. Require exactly one `quality`, one `contract`, and one `merge approval` context, no same-name Actions jobs for the two custom contexts, least-privilege caller permissions, and no failed caller workflow left operative after recovery.
8. If the integration proof exposes a consumer-visible correctness, trust-boundary, or publication blocker, repair it under Issue #3 authority, produce a new release candidate, and repeat Steps 5–7. Record usability and latency improvements separately; they do not block v0.1.0.
9. Obtain an owner comment on Issue #3 approving a `v*` repository ruleset that targets tags and blocks update and deletion. Apply it, inspect the live ruleset, and verify those protections before creating any release tag.
10. Obtain a separate owner comment on Issue #3 approving the exact verified candidate SHA for release. After approval, create annotated tag `v0.1.0` and publish GitHub Release notes with `Added`, `Changed`, `Fixed`, `Compatibility`, `Upgrade`, `Known limitations`, and `Rollback` sections.
11. Verify the annotated tag and GitHub Release target the approved commit. For every `contracts/v0.1.0/*.schema.json`, fetch its `$id` and compare the returned JSON with the tagged file. Verify release immutability is enabled and the release notes name the full consumer pin. Close Issue #3 only after all checks pass.

## Post-release handoff

The release does not authorize these mutations. Open separate Issues and obtain their required approvals before work begins:

1. Implement canonical authority-record rendering and posting with production-parser preflight.
2. Reduce policy latency without changing v0.1.0 semantics or check contexts.
3. Adopt repo-ops in LecturAL production, pinning the published full commit SHA. Approve LecturAL ruleset or enforcement changes separately and observe the first five real pull requests before declaring adoption stable.

## Verification and recovery

The plan-only pull request must pass `go test ./...`, `git diff --check`, base-owned `contract`, and exact-head `merge approval`. The release rehearsal runs before the real release branch and must leave the source checkout unchanged.

The release pull request diff contains only the generated `CHANGELOG.md` and deletion of the consumed fragments. Its PR body carries the release record and authority metadata; those are not repository files. Compare the consumed set with the pre-fold directory listing and require green post-merge quality on the exact candidate.

The LecturAL proof is consumer evidence, not adoption authority. Preserve the dedicated Issue and closed, unmerged pull request as evidence with the fixed head SHA, caller workflow runs, check-run transitions, and permissions. Close it without merge if release is blocked or the candidate changes.

If plan review, folding, release review, post-merge CI, consumer proof, tag protection, or exact-commit approval fails, stop before tagging and repair the applicable branch. If a candidate changes, invalidate all earlier consumer and release approvals. After publication, preserve the immutable tag and publish a corrective version rather than rewriting history.
