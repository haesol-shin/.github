from __future__ import annotations

import copy
import importlib.util
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
VALIDATOR_PATH = ROOT / "actions" / "repository-policy" / "validate.py"
SPEC = importlib.util.spec_from_file_location("repo_ops_validator", VALIDATOR_PATH)
assert SPEC and SPEC.loader
validator = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = validator
SPEC.loader.exec_module(validator)


class ValidatorConformanceTests(unittest.TestCase):
    contract_root = ROOT / "contracts" / "v0.1.0"
    fixtures = ROOT / "fixtures"

    def findings(self, name: str) -> list[str]:
        state, _ = validator.load_fixture(self.fixtures / name)
        return validator.validate_state(state, self.contract_root)

    def test_valid_low_direct_route(self) -> None:
        self.assertEqual([], self.findings("valid-low-direct.json"))

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
