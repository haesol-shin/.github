from __future__ import annotations

import copy
import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
VALIDATOR_PATH = ROOT / "actions" / "repository-policy" / "validate.py"
SPEC = importlib.util.spec_from_file_location("repo_ops_validator", VALIDATOR_PATH)
assert SPEC and SPEC.loader
validator = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = validator
SPEC.loader.exec_module(validator)

CHANGELOG_SPEC = importlib.util.spec_from_file_location(
    "repo_ops_changelog",
    ROOT / "actions" / "repository-policy" / "changelog.py",
)
assert CHANGELOG_SPEC and CHANGELOG_SPEC.loader
changelog = importlib.util.module_from_spec(CHANGELOG_SPEC)
sys.modules[CHANGELOG_SPEC.name] = changelog
CHANGELOG_SPEC.loader.exec_module(changelog)


class ValidatorConformanceTests(unittest.TestCase):
    contract_root = ROOT / "contracts" / "v0.1.0"
    fixtures = ROOT / "fixtures"

    def findings(self, name: str) -> list[str]:
        state, _ = validator.load_fixture(self.fixtures / name)
        return validator.validate_state(state, self.contract_root)

    def v1_comment(
        self,
        *,
        issue: int,
        outcome: str,
        risk: str,
        supersedes: str,
    ) -> tuple[str, str]:
        digest = validator.canonical_digest(
            {
                "issue": issue,
                "outcome": validator.normalize_text(outcome),
                "risk": risk,
            }
        )
        body = (
            "**Intent accepted**\n\n"
            f"{outcome}\n\n"
            f"- Risk: `{risk}`\n"
            f"- Intent digest: `{digest}`\n"
            f"- Supersedes: `{supersedes}`\n\n"
            "Authoritative machine-readable record:\n\n"
            "```text\n"
            f"repo-ops.intent.v1 decision:accepted risk:{risk} "
            f"intent:{digest} supersedes:{supersedes}\n"
            "```\n"
        )
        return body, digest

    def v1_state(self) -> dict:
        state, _ = validator.load_fixture(self.fixtures / "valid-v1-related.json")
        return state

    def test_valid_v1_related_route(self) -> None:
        self.assertEqual([], validator.validate_state(self.v1_state(), self.contract_root))

    def test_v1_digest_is_issue_bound_and_display_values_are_checked(self) -> None:
        state = self.v1_state()
        state["issue"] = 43
        findings = validator.validate_state(state, self.contract_root)
        self.assertIn(
            "accepted-intent digest does not match its canonical issue-bound payload",
            findings,
        )

        state["intent_comments"][1]["body"] = state["intent_comments"][1]["body"].replace(
            "- Risk: `high`",
            "- Risk: `medium`",
            1,
        )
        self.assertIn(
            "accepted-intent displayed risk does not match its raw record",
            validator.validate_state(state, self.contract_root),
        )
        for field, replacement, expected in (
            ("Intent digest", "sha256:" + "0" * 64, "displayed digest"),
            ("Supersedes", "none", "displayed supersession"),
        ):
            state = self.v1_state()
            parsed, error = validator.parse_intent(
                state["intent_comments"][1]["body"],
                issue=42,
                contract_root=self.contract_root,
            )
            self.assertIsNone(error)
            assert parsed is not None
            original = parsed["digest"] if field == "Intent digest" else parsed["supersedes"]
            state["intent_comments"][1]["body"] = state["intent_comments"][1][
                "body"
            ].replace(f"- {field}: `{original}`", f"- {field}: `{replacement}`", 1)
            self.assertIn(
                f"accepted-intent {expected} does not match its raw record",
                validator.validate_state(state, self.contract_root),
            )

    def test_v1_migrates_legacy_and_requires_supersession(self) -> None:
        state = self.v1_state()
        body = state["intent_comments"][1]["body"]
        state["intent_comments"][1]["body"] = body.replace(
            "supersedes:sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871",
            "supersedes:none",
        ).replace(
            "- Supersedes: `sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871`",
            "- Supersedes: `none`",
        )
        self.assertIn(
            "repo-ops.intent.v1 supersedes does not match the current intent",
            validator.validate_state(state, self.contract_root),
        )

    def test_first_v1_without_legacy_uses_none(self) -> None:
        state = self.v1_state()
        state["intent_comments"].pop(0)
        body = state["intent_comments"][0]["body"]
        state["intent_comments"][0]["body"] = body.replace(
            "supersedes:sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871",
            "supersedes:none",
        ).replace(
            "- Supersedes: `sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871`",
            "- Supersedes: `none`",
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))

    def test_v1_replacements_form_one_chain_head(self) -> None:
        state = self.v1_state()
        first = validator.parse_intent(state["intent_comments"][1]["body"], 42)[0]
        assert first is not None
        second_body, second_digest = self.v1_comment(
            issue=42,
            outcome="The trusted validator remains isolated from pull request code.",
            risk="high",
            supersedes=first["digest"],
        )
        state["intent_comments"].append(
            {"author": "maintainer", "association": "OWNER", "body": second_body}
        )
        state["comments"][0]["body"] = state["comments"][0]["body"].replace(
            first["digest"],
            second_digest,
        )
        state["comments"][1]["body"] = state["comments"][1]["body"].replace(
            first["digest"],
            second_digest,
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))

        broken_body, _ = self.v1_comment(
            issue=42,
            outcome="A broken replacement must not become current authority.",
            risk="high",
            supersedes="none",
        )
        state["intent_comments"].append(
            {"author": "maintainer", "association": "OWNER", "body": broken_body}
        )
        self.assertIn(
            "repo-ops.intent.v1 supersedes does not match the current intent",
            validator.validate_state(state, self.contract_root),
        )

    def test_malformed_v1_record_is_rejected(self) -> None:
        state = self.v1_state()
        state["intent_comments"][1]["body"] = state["intent_comments"][1]["body"].replace(
            "decision:accepted",
            "decision:approved",
            1,
        )
        self.assertTrue(
            any(
                "repo-ops.intent.v1" in finding
                for finding in validator.validate_state(state, self.contract_root)
            )
        )

    def test_authority_links_are_single_and_closes_remains_supported(self) -> None:
        state = self.v1_state()
        state["pull_request"]["body"] = state["pull_request"]["body"].replace(
            "Related #42",
            "Fixes #42\nCloses #42",
            1,
        )
        self.assertTrue(
            any(
                "exactly one standalone issue authority line" in finding
                for finding in validator.validate_state(state, self.contract_root)
            )
        )

        legacy = validator.load_fixture(self.fixtures / "valid-high.json")[0]
        legacy["pull_request"]["body"] = legacy["pull_request"]["body"].replace(
            "Fixes #42",
            "Closes #42",
            1,
        )
        self.assertEqual([], validator.validate_state(legacy, self.contract_root))

    def test_authority_parsing_ignores_prose_and_fenced_route_examples(self) -> None:
        state = self.v1_state()
        state["pull_request"]["body"] = state["pull_request"]["body"].replace(
            "Add the trusted repository policy validator.",
            "Fixes the flaky scheduler race.\n"
            "Closes the gap between plan and review.\n"
            "Related work is tracked separately.\n\n"
            "Add the trusted repository policy validator.",
            1,
        )
        state["pull_request"]["body"] += (
            "\n```markdown\nDirect low-risk PR: documentation-only update\n```\n"
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))

        body = state["pull_request"]["body"] + "\nFixes #not-an-issue\n"
        self.assertEqual(42, validator.parse_related_issue(body))
        self.assertTrue(validator.parse_authority_links(body)[1])

    def test_markdown_code_cannot_hide_or_invent_authority(self) -> None:
        visible = "Related #42\n``` x ```\nFixes #99\n"
        links, errors = validator.parse_authority_links(visible)
        self.assertEqual([42, 99], [link["issue"] for link in links])
        self.assertTrue(
            any("exactly one standalone issue authority line" in error for error in errors)
        )

        indented = (
            "Related #42\n\n"
            "    Fixes #99\n"
            "    Direct low-risk PR: example only\n"
        )
        self.assertEqual(42, validator.parse_related_issue(indented))
        self.assertFalse(validator.is_direct_low_risk(indented))

    def test_historical_intent_digest_discussion_is_not_a_legacy_record(self) -> None:
        self.assertEqual([], self.findings("valid-v1-historical-discussion.json"))

    def test_changelog_declaration_requires_owned_valid_fragment(self) -> None:
        state = self.v1_state()
        state["pull_request"]["body"] = state["pull_request"]["body"].replace(
            "Changelog: not-required — this fixture exercises receipt validation without a release-note change",
            "Changelog: required — this change needs a release note",
            1,
        )
        state["files"] = [
            {"filename": "changelog.d/42-validator.md", "status": "added"},
        ]
        state["file_contents"] = {
            "changelog.d/42-validator.md": "## Changed\n- Validate the trusted policy contract.\n"
        }
        self.assertEqual([], validator.validate_state(state, self.contract_root))

        state["files"][0]["filename"] = "changelog.d/41-validator.md"
        self.assertIn(
            "issue-backed fragment filename must begin with the linked issue number",
            validator.validate_state(state, self.contract_root),
        )

    def test_fragment_parser_rejects_empty_bullets_and_unsupported_headings(self) -> None:
        with self.assertRaises(ValueError):
            validator._changelog_helpers()(
                "## Added\n- \n",
                filename="empty.md",
            )
        with self.assertRaises(ValueError):
            validator._changelog_helpers()(
                "## Notes\n- Unsupported.\n",
                filename="unsupported.md",
            )

    def test_contract_check_is_independent_of_exact_head_merge_receipt(self) -> None:
        state = self.v1_state()
        state["comments"] = state["comments"][:1]
        self.assertEqual(
            [],
            validator.validate_state(state, self.contract_root, check="contract"),
        )
        self.assertIn(
            "no merge-ready receipt was found",
            validator.validate_state(state, self.contract_root),
        )

    def test_high_risk_plan_approval_requires_owner_authority(self) -> None:
        state = self.v1_state()
        state["comments"][0]["association"] = "MEMBER"
        self.assertIn(
            "high-risk plan approval was not posted by an authorized maintainer",
            validator.validate_state(state, self.contract_root, check="contract"),
        )

    def test_plan_approval_requires_display_wrapper(self) -> None:
        state = self.v1_state()
        state["comments"][0]["body"] = (
            "repo-ops.plan-approval.v1 decision:approved risk:high "
            "issue:42 intent:sha256:0dbcd01f3a688d6e6c688048de2f13c525e3f6df968573ae7f5c4fbbdeae59dc "
            "plan:sha256:" + "d" * 64 + " plan-commit:" + "d" * 40
        )
        self.assertIn(
            "malformed repo-ops.plan-approval.v1 display wrapper",
            validator.validate_state(state, self.contract_root, check="contract"),
        )

    def test_repository_permissions_are_required_for_record_authority(self) -> None:
        for permission in ("read", "write"):
            with self.subTest(permission=permission):
                self.assertFalse(
                    validator.record_authorized(
                        {"association": "MEMBER", "permission": permission},
                        record="repo-ops.merge-review.v1",
                    )
                )
        for permission in ("maintain", "admin"):
            with self.subTest(permission=permission):
                self.assertTrue(
                    validator.record_authorized(
                        {"association": "COLLABORATOR", "permission": permission},
                        record="repo-ops.plan-approval.v1",
                        risk="medium",
                    )
                )
        self.assertFalse(
            validator.record_authorized(
                {"association": "MEMBER", "permission": "admin"},
                record="repo-ops.plan-approval.v1",
                risk="high",
            )
        )



    def test_fenced_intent_examples_are_not_authority(self) -> None:
        examples = (
            "```text\n"
            "repo-ops.intent.v1 decision:accepted risk:high "
            "intent:sha256:<digest> supersedes:<previous>\n"
            "```\n",
            "```text\nIntent accepted.\nIntent digest: sha256:<digest>\n```\n",
        )
        for body in examples:
            self.assertEqual(
                (None, None),
                validator.parse_intent(
                    body,
                    issue=42,
                    contract_root=self.contract_root,
                ),
            )


    def test_valid_low_direct_route(self) -> None:
        self.assertEqual([], self.findings("valid-low-direct.json"))

    def test_low_risk_plan_is_optional_authority(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-low-direct.json")
        digest = "sha256:" + "d" * 64
        state["plan_digest"] = digest
        state["comments"][0]["body"] = state["comments"][0]["body"].replace(
            "plan:none",
            f"plan:{digest}",
            1,
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))


    def test_folding_is_stable_duplicate_safe_and_consumes_only_fragments(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fragments = root / "changelog.d"
            fragments.mkdir()
            (fragments / "3-z-last.md").write_text(
                "## Added\n- Z entry.\n",
                encoding="utf-8",
            )
            (fragments / "3-a-first.md").write_text(
                "## Added\n- A entry.\n",
                encoding="utf-8",
            )
            unrelated = root / "keep.txt"
            unrelated.write_text("keep", encoding="utf-8")
            changelog_path = root / "CHANGELOG.md"
            consumed = changelog.fold_changelog(
                fragments,
                changelog_path,
                version="v0.1.0",
                date="2026-09-21",
            )
            self.assertEqual(["3-a-first.md", "3-z-last.md"], [path.name for path in consumed])
            self.assertEqual(
                "## [v0.1.0] - 2026-09-21\n\n"
                "### Added\n"
                "- A entry.\n"
                "- Z entry.\n",
                changelog_path.read_text(encoding="utf-8"),
            )
            self.assertFalse((fragments / "3-a-first.md").exists())
            self.assertFalse((fragments / "3-z-last.md").exists())
            self.assertEqual("keep", unrelated.read_text(encoding="utf-8"))

            (fragments / "3-again.md").write_text(
                "## Fixed\n- A fix.\n",
                encoding="utf-8",
            )
            with self.assertRaises(changelog.ChangelogError):
                changelog.fold_changelog(
                    fragments,
                    changelog_path,
                    version="v0.1.0",
                    date="2026-09-21",
                )
            self.assertTrue((fragments / "3-again.md").exists())

    def test_valid_high_risk_route(self) -> None:
        self.assertEqual([], self.findings("valid-high.json"))

    def test_every_required_pull_request_heading_is_enforced(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-low-direct.json")
        for heading in validator.CONTRACT["pull_request"]["required_headings"]:
            with self.subTest(heading=heading):
                candidate = copy.deepcopy(state)
                candidate["pull_request"]["body"] = candidate["pull_request"]["body"].replace(
                    heading, f"Removed {heading}", 1
                )
                self.assertIn(
                    f"pull request body is missing {heading}",
                    validator.validate_state(candidate, self.contract_root),
                )

    def test_every_required_issue_heading_is_enforced(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-high.json")
        for heading in validator.CONTRACT["issue"]["required_headings"]:
            with self.subTest(heading=heading):
                candidate = copy.deepcopy(state)
                candidate["issue_body"] = candidate["issue_body"].replace(
                    heading, f"Removed {heading}", 1
                )
                self.assertIn(
                    f"linked issue body is missing {heading}",
                    validator.validate_state(candidate, self.contract_root),
                )

    def test_repository_specific_headings_are_allowed(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-high.json")
        state["issue_body"] = state["issue_body"].replace(
            "## Context",
            "## Security boundary\n\nNo production credentials.\n\n## Context",
            1,
        )
        state["pull_request"]["body"] = state["pull_request"]["body"].replace(
            "## Verification",
            "## Contract impact\n\nNo public schema change.\n\n## Verification",
            1,
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))

    def test_required_headings_are_unique_and_ordered(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-low-direct.json")
        duplicate = copy.deepcopy(state)
        duplicate["pull_request"]["body"] += "\n## Summary\n\nDuplicate.\n"
        self.assertIn(
            "pull request body contains duplicate ## Summary",
            validator.validate_state(duplicate, self.contract_root),
        )

        reordered = copy.deepcopy(state)
        body = reordered["pull_request"]["body"]
        body = body.replace("## Summary", "## Temporary", 1)
        body = body.replace("## Changes", "## Summary", 1)
        body = body.replace("## Temporary", "## Changes", 1)
        reordered["pull_request"]["body"] = body
        self.assertIn(
            "pull request body required headings must appear in contract order",
            validator.validate_state(reordered, self.contract_root),
        )

    def test_fenced_template_examples_are_not_counted_as_headings(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-high.json")
        state["pull_request"]["body"] += (
            "\n```markdown\n## Summary\n\nQuoted template.\n\n## Changes\n```\n"
        )
        state["issue_body"] += (
            "\n~~~markdown\n## Problem\n\nQuoted template.\n\n## Context\n~~~\n"
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))

    def test_commonmark_indented_headings_are_normalized(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-low-direct.json")
        state["pull_request"]["body"] = state["pull_request"]["body"].replace(
            "## Impact", "   ## Impact", 1
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))

    def test_invalid_fixtures_fail_for_declared_reason(self) -> None:
        for path in sorted(self.fixtures.glob("invalid-*.json")):
            with self.subTest(path=path.name):
                state, expected = validator.load_fixture(path)
                findings = validator.validate_state(state, self.contract_root)
                self.assertTrue(findings)
                self.assertTrue(
                    any(expected in finding for finding in findings),
                    f"{path.name}: expected {expected!r} in {findings!r}",
                )

    def test_distinct_identity_requires_native_review(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-low-direct.json")
        state = copy.deepcopy(state)
        state["policy"]["review"]["shared_identity"] = False
        state["comments"][0]["author"] = "review-bot"

        findings = validator.validate_state(state, self.contract_root)
        self.assertIn(
            "independent review receipt must be submitted as a native GitHub Review",
            findings,
        )

        state["reviews"].append(state["comments"].pop())
        self.assertEqual([], validator.validate_state(state, self.contract_root))

    def test_untrusted_record_comments_cannot_block_or_approve(self) -> None:
        state, _ = validator.load_fixture(self.fixtures / "valid-low-direct.json")
        state = copy.deepcopy(state)
        state["comments"].insert(
            0,
            {
                "author": "external",
                "association": "NONE",
                "body": "repo-ops.merge-review.v1 malformed",
            },
        )
        self.assertEqual([], validator.validate_state(state, self.contract_root))

        state["comments"][-1]["association"] = "NONE"
        self.assertIn(
            "no merge-ready receipt was found",
            validator.validate_state(state, self.contract_root),
        )

    def test_plan_path_accepts_canonical_repository_paths(self) -> None:
        expected = ".ops/plans/42-validator.md"
        self.assertEqual(expected, validator.parse_plan_path(f"Plan: {expected}"))
        self.assertEqual(expected, validator.parse_plan_path(f"Plan: ./{expected}"))
        self.assertEqual(
            expected,
            validator.parse_plan_path(
                "Plan: [reviewed plan](https://github.com/owner/repo/blob/main/"
                ".ops/plans/42-validator.md)"
            ),
        )

    def test_contract_templates_and_schemas_stay_aligned(self) -> None:
        pull_request_template = (ROOT / ".github" / "PULL_REQUEST_TEMPLATE.md").read_text(
            encoding="utf-8"
        )
        issue_template = (
            ROOT / ".github" / "ISSUE_TEMPLATE" / "work_item.md"
        ).read_text(encoding="utf-8")
        for heading in validator.CONTRACT["pull_request"]["required_headings"]:
            self.assertIn(heading, pull_request_template)
        for heading in validator.CONTRACT["issue"]["required_headings"]:
            self.assertIn(heading, issue_template)

        for record, definition in validator.CONTRACT["records"].items():
            schema = validator.json.loads(
                (self.contract_root / definition["schema"]).read_text(encoding="utf-8")
            )
            self.assertEqual(record, schema["properties"]["record"]["const"])
            self.assertEqual(
                definition["field_order"],
                [field for field in schema["required"] if field != "record"],
            )

        policy = validator.yaml.safe_load(
            (ROOT / ".github" / "repo-policy.yml").read_text(encoding="utf-8")
        )
        self.assertEqual(
            [],
            validator.validate_schema(
                policy,
                self.contract_root / validator.CONTRACT["repository_policy_schema"],
                "repository policy",
            ),
        )
        for schema_path in self.contract_root.glob("*.schema.json"):
            schema = validator.json.loads(schema_path.read_text(encoding="utf-8"))
            self.assertEqual(
                "https://raw.githubusercontent.com/haesol-shin/.github/v0.1.0/"
                f"contracts/v0.1.0/{schema_path.name}",
                schema["$id"],
            )
        workflow = (ROOT / ".github" / "workflows" / "repository-policy.yml").read_text(
            encoding="utf-8"
        )
        self.assertIn("name: Repository policy / contract", workflow)
        self.assertIn("name: Repository policy / merge approval", workflow)
        self.assertIn("needs: [revoke, contract]", workflow)
        self.assertIn("if: ${{ always() }}", workflow)
        policy_workflow = (ROOT / ".github" / "workflows" / "policy.yml").read_text(
            encoding="utf-8"
        )
        self.assertIn("name: Repository policy / contract", policy_workflow)
        self.assertIn("name: Repository policy / merge approval", policy_workflow)
        self.assertIn("uses: ./.repo-ops/actions/repository-policy", policy_workflow)
        self.assertNotIn("uses: ./.github/workflows/repository-policy.yml", policy_workflow)
        self.assertIn("repo-ops.intent.v1", policy_workflow)
        self.assertIn("checks: write", policy_workflow)
        quality_workflow = (ROOT / ".github" / "workflows" / "ci.yml").read_text(
            encoding="utf-8"
        )
        self.assertIn("  quality:\n    name: quality", quality_workflow)
        self.assertIn('-f "head_sha=${HEAD_SHA}"', policy_workflow)
        self.assertIn('-f "conclusion=${conclusion}"', policy_workflow)
        self.assertIn("needs.revoke.result == 'success'", policy_workflow)

    def test_record_rejects_noncanonical_spacing(self) -> None:
        record, error = validator.parse_record_line(
            "repo-ops.merge-review.v1  verdict:merge-ready",
            "repo-ops.merge-review.v1",
        )
        self.assertIsNone(record)
        self.assertEqual(
            "repo-ops.merge-review.v1 fields must use one ASCII space",
            error,
        )


if __name__ == "__main__":
    unittest.main()
