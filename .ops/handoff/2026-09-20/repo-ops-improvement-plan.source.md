# Repository policy delivery plan

## Goal

Publish `repo-ops/v0.1.0` on the shortest safe path to LecturAL consumption. Consumer-facing behavior must be proven from LecturAL before the immutable release. Maintainer-only tooling must not delay the release unless the consumer test shows it is required.

## Current critical path

1. Merge the approved Issue #5 plan-only pull request after separate merge approval.
2. Complete the three Issue #5 Go migration stages: semantic evaluator, deterministic collector/replay evidence, then clean production cutover.
3. Revise and re-approve the committed Issue #3 release plan against the exact post-cutover state.
4. Produce one exact release-candidate commit with complete central conformance and CI evidence.
5. Open a non-merged LecturAL integration pull request pinned to the full candidate commit SHA.
6. Exercise the consumer-facing reusable workflow from LecturAL before any tag or GitHub Release.
7. Fix only consumer-facing failures exposed by that integration gate, then repeat against the new exact candidate.
8. Obtain separate approval for the exact release commit, protect the release tag, publish `v0.1.0`, and let LecturAL pin the published full commit SHA.

## LecturAL integration gate

The non-merged LecturAL pull request must prove:

- the reusable workflow loads from the exact candidate SHA;
- immutable-base code and policy are used;
- machine contexts are `quality`, `contract`, and `merge approval`;
- no Actions job duplicates the custom `contract` or `merge approval` context names;
- policy denial produces failing custom contexts without a misleading transport result;
- valid authority recovery on the same PR head transitions the custom contexts correctly;
- no operative stale transport failure remains on the consumer PR;
- permissions and exact-head publication remain fail-closed.

The integration branch is evidence only until the immutable release is approved. It must not merge against a mutable branch, mutable tag, or unapproved candidate.

## Deferred operational hardening

A maintainer-only authority publisher remains useful but is not a release prerequisite:

- record-specific typed builders for intent, plan approval, and merge review;
- one canonical external-git diff adapter;
- known-answer and cross-type negative cases;
- live-state fingerprints around exact-byte publication;
- immutable record posting, post-write read-back, and identical-retry no-op;
- serialized, recoverable multi-check publication.

Create separate Issue authority before implementing this tooling. Promote only the consumer-facing subset into the release critical path if the LecturAL integration gate demonstrates that it is necessary.

## Boundaries

- Issue #5 owns the Go validator migration.
- Issue #3 owns release-candidate verification, the LecturAL pre-release integration gate, tag approval, and publication.
- The local authority publisher is not folded into Issue #5 or treated as a prerequisite without new evidence.
- No ruleset, tag, release, or LecturAL merge occurs without its existing explicit approval gate.
