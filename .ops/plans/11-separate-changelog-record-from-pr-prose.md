# Separate changelog authority from pull-request prose

## Goal

Replace the unreleased presentation-sensitive pull-request changelog declaration with one versioned machine record before the Go validator freezes the contract. Human headings, bullets, and surrounding explanation remain advisory. Changelog intent remains explicit, unique, deterministic, and fail-closed.

The new record is one unfenced line:

```text
repo-ops.changelog.v1 kind:<required|not-required|release> value:<token>
```

Examples:

```text
repo-ops.changelog.v1 kind:required value:fragment-added
repo-ops.changelog.v1 kind:not-required value:plan-only
repo-ops.changelog.v1 kind:release value:v0.1.0
```

Issue #11 is authoritative. Its accepted high-risk intent is `sha256:f46a76e2f04f6aae15aa545c7ddebb039a5744b004cbc2fde400915289057d0a`.

## Boundaries

This repair changes only how a pull request declares changelog intent. It preserves fragment requirements, issue/direct filename ownership, fragment syntax, direct `CHANGELOG.md` protection, release-only fragment consumption, plan authority, review authority, exact-head binding, risk classification, check names, workflow trust boundaries, rulesets, and release state.

The repair does not ship Go, change production workflow topology, infer changelog intent from a diff, accept multiple declarations, introduce a compatibility alias, tag, release, or adopt a consumer. `repo-ops/v0.1.0` is unreleased and has no consumer, so the change is a clean cutover: repository templates, fixtures, tests, plans that show current syntax, and open pull requests migrate to the versioned record.

PR #10 stays paused. Its uncommitted Stage 1 files remain isolated and are neither committed nor pushed until this repair merges, PR #10 is rebased, its body uses the new record, and its base-owned `contract` check passes.

## Contract

Add `repo-ops.changelog.v1` to `contracts/v0.1.0/contract.json` with field order `["kind", "value"]` and schema `changelog-declaration.schema.json`. The serialized first token is the record identifier; the normalized schema instance adds it as `record`. Thus the line has exactly one identifier plus two `key:value` fields, while the schema object has exactly three properties:

- `record`: constant `repo-ops.changelog.v1`
- `kind`: one of `required`, `not-required`, `release`
- `value`: one ASCII token

The schema is Draft 2020-12, `type: object`, `additionalProperties: false`, and `required: ["record", "kind", "value"]`. Each property has `type: string`; `record` uses `const`, `kind` uses `enum`, and `value` uses an `if`/`then` branch so release versions and reason codes use the exact patterns below.

Remove the legacy `pull_request.changelog.declaration_prefix`, `values`, `required_rationale`, and `release_version_pattern` keys from `contract.json`. The record registry and `contracts/v0.1.0/changelog-declaration.schema.json` become the only declaration-shape authority; existing lifecycle behavior remains owned by repository policy plus `validate_changelog_state`.

For `required` and `not-required`, `value` must match ASCII `^[a-z0-9]+(?:-[a-z0-9]+)*$` and must not equal `replace-me`. It is the required reason code, replacing the old free-form rationale; no free-form prose is authoritative or required. For `release`, `value` must match `^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`; prerelease/build suffixes and leading-zero components are invalid.

Candidate detection reuses `unfenced_lines` exactly: CommonMark fences of three or more backticks or tildes are tracked by marker character and opening length; every line inside is ignored. Outside fences, trim leading and trailing whitespace, then select every line whose first whitespace-delimited token is exactly `repo-ops.changelog.v1`. Indentation is accepted. A candidate is canonical only when the identifier, `kind`, and `value` are separated by one ASCII space and there are no tabs, extra internal spaces, missing fields, duplicate fields, unknown fields, or trailing tokens. Exactly one valid candidate is required. A candidate count other than one or any malformed candidate returns structural findings before lifecycle validation.

Structural findings are stable and single-result: parsing stops at the first applicable row.

| Precedence | Condition | Finding |
| --- | --- | --- |
| 1 | zero candidates | `pull request body must contain exactly one repo-ops.changelog.v1 record` |
| 2 | two or more candidates, including valid plus malformed | `pull request body must contain exactly one repo-ops.changelog.v1 record` |
| 3 | one candidate with noncanonical spacing, field count/order, duplicate/unknown key, or trailing token | `malformed repo-ops.changelog.v1 record` |
| 4 | normalized object fails schema, including kind, reason, placeholder, or version | existing structured schema identity for label `changelog declaration`, instance path, and keyword |

Python library message wording for row 4 is presentation, consistent with the Go migration contract; label/path/keyword and ordering are authoritative.

The legacy `- Changelog: … — …` line is ordinary prose, never a candidate, and receives no alias or fallback. Legacy-only bodies fail for a missing v1 record; legacy prose alongside one valid v1 record is harmless; a valid v1 record plus another malformed v1 candidate fails. Templates and repository-owned active guidance stop emitting the legacy syntax.

Parsing returns either one normalized declaration `{kind, value}` or structural findings. Only a normalized declaration reaches lifecycle validation:

- `required` needs at least one added or modified valid fragment; its reason code does not select a path.
- `not-required` means no fragment is required solely by the declaration. It does not permit invalid fragments, direct `CHANGELOG.md` edits, ownership violations, fragment deletion, or release-only operations; an otherwise valid added fragment remains permitted.
- `release` requires a `CHANGELOG.md` update, at least one deleted fragment, no modified fragment, and the semantic version from `value`.
- issue-backed and direct-route fragment ownership rules remain unchanged.

## Implementation

1. Create `contracts/v0.1.0/changelog-declaration.schema.json` with `$id` `https://raw.githubusercontent.com/haesol-shin/.github/v0.1.0/contracts/v0.1.0/changelog-declaration.schema.json`. Register `repo-ops.changelog.v1` and field order `["kind", "value"]` in `contracts/v0.1.0/contract.json`; remove the competing legacy declaration-shape keys; align schema-ID and contract/template tests.
2. Replace `CHANGELOG_DECLARATION_PATTERN` and `parse_changelog_declaration` with the common record boundary over `unfenced_lines`. Structural parsing returns `{kind, value}` or findings before `validate_changelog_state` runs. Candidate-count errors precede malformed-field/schema errors, which precede lifecycle findings.
3. Update `.github/PULL_REQUEST_TEMPLATE.md` to contain one active placeholder, `repo-ops.changelog.v1 kind:not-required value:replace-me`, which deliberately fails until edited. Put the three valid alternatives and instructions in a fenced `text` example. Update `changelog.d/README.md` to name the v1 release record.
4. Migrate validator state fixtures to v1 while preserving every non-changelog byte and expected lifecycle finding. Change `fixtures/changelog-conformance.json` from `input.declaration` to `input.body` so it exercises complete PR-body parsing. Add body-level cases for arbitrary headings/prose/Unicode punctuation, indentation, fenced-only, legacy-only, legacy-plus-valid, valid-plus-malformed, duplicate records, wrong order, missing/duplicate/unknown fields, tabs/multiple spaces, trailing tokens, invalid kind, invalid reason code, the `replace-me` placeholder, and invalid release versions.
5. Search `.github`, `contracts`, `actions/repository-policy`, `fixtures`, `tests`, `changelog.d`, and `.ops/plans` for both `Changelog:` and `repo-ops.changelog`. Migrate executable fixtures/tests and active guidance. Update the current/future procedures in plans 3, 5, and 8; leave a legacy occurrence only when it describes immutable past evidence, and list each exclusion with its reason in the PR evidence.
6. Add `changelog.d/11-changelog-machine-record.md` containing `## Changed` and `- Replace the presentation-sensitive pull-request changelog declaration with a versioned machine record.` Because live validation checks out the immutable `main` validator, the repair PR body uses `- Changelog: required — replace the presentation-sensitive declaration` through merge and contains no active v1 record. The exact repair head must pass base-owned `Repository policy / contract`; it never relies on the candidate parser to authorize itself.
7. Run `python -m unittest discover -s tests -v` and `git diff --check`. Directly run `actions/repository-policy/validate.py --fixture` on temporary body-level states: two valid `required` states differing only in prose/headings/punctuation must produce identical results; missing, duplicate, legacy-only, and malformed v1 candidates must exit 1 with the structural result specified above before lifecycle findings.
8. Under the immutable base contract, obtain an owner `repo-ops.plan-approval.v1`, exact-head `repo-ops.merge-review.v1`, green `Repository policy / contract`, and green `Repository policy / merge approval`; the schemas and field orders registered in `contract.json` remain the single source for receipt grammar. A later push requires a new exact-head receipt. Squash-merge the complete repair as one rollback unit. After merge, verify post-merge `CI / quality`. Then rebase PR #10 onto that exact main commit, replace its body declaration with `repo-ops.changelog.v1 kind:required value:go-validator`, regenerate `tests/testdata/go-oracle` and the differential corpus from the repaired Python oracle, update the Go parser/tests, and require zero Python/Go divergence plus green base-owned `contract` before Stage 1 resumes. Old-base goldens are invalid after this repair.

## Verification

The permanent suite must prove observable policy behavior rather than parser internals:

- Semantically identical records pass under unrelated prose, heading, bullet, and Unicode punctuation changes.
- Fenced examples do not satisfy or duplicate the active record.
- Legacy-only, missing, duplicate, malformed, reordered, missing-field, duplicate-field, unsupported-field, internal-whitespace, trailing-token, placeholder, invalid-kind, invalid-reason, and invalid-release-version bodies fail with deterministic structural findings before lifecycle findings.
- `required`, `not-required`, and `release` preserve the lifecycle outcomes defined above; changing a valid reason code alone does not change lifecycle results.
- The template contains one active rejected placeholder and only fenced valid alternatives.
- Generic headings remain advisory.
- Authority, intent, plan, merge-review, quality, and exact-head behavior is unchanged; migrated fixtures differ only where the declaration contract requires it.

Direct smoke uses the trusted Python entrypoint against temporary fixture JSON; no live GitHub state is the oracle. A cold review compares Issue #11, this plan, `contract.json`, the schema, template, parser, fixtures, and tests. Approval requires no compatibility alias, no prose-format dependency, and no weakening of changelog lifecycle rules.

## Rollback

Before merge, close the repair PR and leave PR #10 paused. After merge but before any consumer release, revert the one squash-merge commit, including schema, registry, parser, template, docs, fixtures, tests, active-plan references, and fragment. PR #10 is the affected open implementation PR: restore its body to the legacy declaration only after the revert is on `main`, rebase it onto that revert, invalidate exact-head approvals through the new commit, keep it paused, and require the restored base contract to pass before resuming work. Never retain both declaration formats: dual parsing would create two sources of truth and ambiguous precedence.

## Acceptance

- Issue #11 has the accepted high-risk intent named above.
- The plan-only head changes only `.ops/plans/11-separate-changelog-record-from-pr-prose.md`; its PR body uses the legacy declaration required by immutable `main`.
- Owner plan approval uses the existing canonical plan digest (`canonical_digest({"text": normalize_text(plan_bytes)})`), names intent `sha256:f46a76e2f04f6aae15aa545c7ddebb039a5744b004cbc2fde400915289057d0a`, and binds the exact 40-hex plan commit in a `repo-ops.plan-approval.v1` comment.
- Implementation removes the legacy contract keys and contains the registered schema, parser cutover, exact template/README migration, full fixture/test migration, active-plan reference migration, and Issue #11 changelog fragment.
- Complete conformance, direct smoke, diff check, and cold review pass.
- Exact-head contract and merge approval are obtained before merge; a later push invalidates the receipt.
- Post-merge `quality` passes.
- PR #10 is rebased, migrated to `repo-ops.changelog.v1`, its Python oracle and Go corpus are regenerated from the repaired base, zero divergence is proven, and base-owned `contract` is green before paused Go work resumes.
