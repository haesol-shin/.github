# Standardize reusable Issue and pull request structure

Issue: #1
Intent: `sha256:9c4e41705f3b854b3c220eb46120cbf73aeadef49a10a52cf6947db7ef292197`

## Approach

Complete the unadopted `repo-ops/v0.1.0` contract in place in `haesol-shin/.github` and align its design source at `haesol-shin/agent-skills:docs/repository-operations.md`.

The Issue and pull request declarations in `contracts/v0.1.0/contract.json` each have this exact shape:

```json
{
  "heading_level": 2,
  "required_heading_occurrences": 1,
  "required_heading_order": true,
  "required_headings": ["complete ordered document-specific list"],
  "allow_additional_headings": true
}
```

The Issue list is `["## Problem", "## Desired outcome", "## Work", "## Acceptance", "## Non-goals", "## Context"]`. The pull request list is `["## Summary", "## Changes", "## Impact", "## Verification", "## Risk and rollback", "## Related"]`. Each required heading appears exactly once. Additional level-two headings, including repeated repository-specific headings, are accepted anywhere without changing the relative order of required headings. Level-three and deeper subsections are outside this top-level contract. A renamed required heading is treated as an allowed additional heading plus a missing required heading.

`actions/repository-policy/validate.py::validate_state` reads those declarations. Missing headings produce `<document label> is missing <heading>`; duplicate required headings produce `<document label> contains duplicate <heading>`; reordered required headings produce `<document label> required headings must appear in contract order`. Because `allow_additional_headings` is true, this revision produces no unsupported-heading diagnostic.

The default templates live at `.github/ISSUE_TEMPLATE/work_item.md` and `.github/PULL_REQUEST_TEMPLATE.md`. Valid and invalid policy states live under `fixtures/`; conformance coverage lives in `tests/test_validator.py`. The matching design documentation change is limited to `haesol-shin/agent-skills:docs/repository-operations.md`.

This changes the draft workflow contract and is therefore high risk under the base policy. It does not alter intent receipts, plan approvals, merge-review receipts, risk classification, release authority, or deployment authority.

## Execution

1. Expand `.github/ISSUE_TEMPLATE/work_item.md` with `Work`, `Acceptance`, `Non-goals`, and `Context`.
2. Expand `.github/PULL_REQUEST_TEMPLATE.md` with `Impact` and `Risk and rollback`.
3. Add the exact heading declarations above to `contracts/v0.1.0/contract.json`.
4. Make `actions/repository-policy/validate.py` enforce the declared level, occurrence, and order while accepting additional level-two sections.
5. Update `fixtures/valid-high.json` and `fixtures/valid-low-direct.json`; add focused coverage in `tests/test_validator.py`.
6. Align `agent-skills/docs/repository-operations.md` through a separate linked pull request after the `.github` implementation is ready.

## Verification and recovery

Run `python -m unittest discover -s tests -v` from the `haesol-shin/.github` repository root. This is the authoritative behavioral gate. It must prove: each missing required heading returns the exact missing-heading diagnostic; a duplicate required heading returns the exact duplicate diagnostic; a changed relative order returns the exact order diagnostic; additional repository-specific level-two headings leave the state valid; `valid-high.json` preserves its existing high-risk issue, intent, plan-approval, merge-review, and quality state and passes; `valid-low-direct.json` preserves its existing low-risk direct authorization, merge-review, and quality state and passes; and default templates contain all declared headings. Run `git diff --check` in both repository roots.

Cold-read the standard, templates, contract, and validator together as a supplemental comprehension check. Record `APPROVE` only when the reader reports no mismatch in Issue/PR responsibilities, heading sets, extension policy, or enforcement ownership. A cold read cannot override a failing conformance test.

Merge the `.github` implementation before the `agent-skills` documentation alignment. The documentation pull request may remain open while `.github` is reviewed, but it must not merge first. Do not adopt `repo-ops/v0.1.0` in a product repository until both merge. If `.github` merges and the documentation change cannot merge, revert `.github` before adoption. If the documentation change merges first, immediately revert the documentation change. For full rollback after both merge, revert the documentation change first and then `.github`. No product repository has adopted this revision, so rollback requires no consumer migration.
