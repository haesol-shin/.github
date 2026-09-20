"""Replay checker for workflow-only intent-revocation bundles.

Does not invoke validate.py or cmd/repo-ops-validator. Does not call live
GitHub or a working-tree git remote.
"""

from __future__ import annotations

import json
import re
import sys
import unittest
from pathlib import Path
from typing import Any
from urllib.parse import urlparse

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[3]
POLICY_WORKFLOW = ROOT / ".github" / "workflows" / "policy.yml"
COMPARE = ROOT / "tests" / "testdata" / "go-oracle" / "compare.py"
VALIDATOR_REPLAY = ROOT / "tests" / "testdata" / "events" / "replay.py"

REQUIRED_BUNDLES = (
    "issue_comment-created-intent",
    "issue_comment-edited-intent",
    "issue_comment-deleted-intent",
)
WORKFLOW_EXPECTED_KEYS = ("job", "linked_open_pull_heads", "check_runs")
CHECK_RUN_KEYS = ("name", "conclusion", "head_sha")
VALIDATOR_EXPECTED_KEYS = (
    "exit",
    "findings",
    "check_conclusion",
    "plan_digest",
    "diff_digest",
)
JOB_PERMISSIONS = {
    "contents": "read",
    "issues": "read",
    "pull-requests": "read",
    "checks": "write",
}
ASSOCIATIONS = {"OWNER", "MEMBER", "COLLABORATOR"}
INTENT_MARKERS = ("repo-ops.intent.v1", "Intent accepted.")
CHECK_NAMES = ("contract", "merge approval")
LINK_RE = re.compile(
    r"(?im)^(Fixes|Closes|Related)\s+#(?P<issue>[1-9][0-9]*)\s*$"
)
JOB_IF_NEEDLES = (
    "github.event_name == 'issue_comment'",
    "!github.event.issue.pull_request",
    "github.event.comment.author_association == 'OWNER'",
    "github.event.comment.author_association == 'MEMBER'",
    "github.event.comment.author_association == 'COLLABORATOR'",
    "contains(github.event.comment.body, 'repo-ops.intent.v1')",
    "contains(github.event.comment.body, 'Intent accepted.')",
    "contains(github.event.changes.body.from, 'repo-ops.intent.v1')",
    "contains(github.event.changes.body.from, 'Intent accepted.')",
)
LEAK_PATTERNS = (
    re.compile(r"ghp_[A-Za-z0-9]{20,}"),
    re.compile(r"github_pat_[A-Za-z0-9_]{20,}"),
    re.compile(r"ghs_[A-Za-z0-9]{20,}"),
    re.compile(r"(?i)bearer\s+(?!redacted\b)\S+"),
    re.compile(r"(?i)x-access-token:(?!redacted\b)\S+"),
)


def load_json(path: Path) -> Any:
    return json.loads(path.read_text(encoding="utf-8"))


def dump_text(path: Path) -> str:
    return path.read_text(encoding="utf-8")


def job_blocks(workflow: str) -> dict[str, str]:
    match = re.search(r"(?m)^jobs:\n", workflow)
    if not match:
        raise AssertionError("policy.yml has no jobs:")
    body = workflow[match.end() :]
    blocks: dict[str, str] = {}
    matches = list(re.finditer(r"(?m)^  ([A-Za-z0-9_-]+):\n", body))
    for index, found in enumerate(matches):
        name = found.group(1)
        start = found.end()
        end = matches[index + 1].start() if index + 1 < len(matches) else len(body)
        blocks[name] = body[start:end]
    return blocks


def job_permissions(block: str) -> dict[str, str]:
    match = re.search(
        r"(?m)^    permissions:\n((?:      [^\n]+\n)+)",
        block,
    )
    if not match:
        raise AssertionError("job is missing permissions")
    parsed: dict[str, str] = {}
    for line in match.group(1).splitlines():
        key, _, value = line.strip().partition(":")
        parsed[key.strip()] = value.strip()
    return parsed


def job_if(block: str) -> str:
    match = re.search(
        r"(?ms)^    if: >-\n(?P<body>(?:      [^\n]*\n)+)",
        block,
    )
    if not match:
        raise AssertionError("job is missing folded if:")
    lines = [line[6:] for line in match.group("body").splitlines()]
    return " ".join(part.strip() for part in lines if part.strip())


def contains_marker(text: str) -> bool:
    return any(marker in (text or "") for marker in INTENT_MARKERS)


def intent_job_selected(event_name: str, event: dict[str, Any]) -> bool:
    if event_name != "issue_comment":
        return False
    issue = event.get("issue") or {}
    if issue.get("pull_request"):
        return False
    comment = event.get("comment") or {}
    if comment.get("author_association") not in ASSOCIATIONS:
        return False
    body = comment.get("body") or ""
    previous = ((event.get("changes") or {}).get("body") or {}).get("from") or ""
    return contains_marker(body) or contains_marker(previous)


def repository_name(event: dict[str, Any]) -> str:
    repo = event.get("repository") or {}
    name = repo.get("full_name")
    if not isinstance(name, str) or "/" not in name:
        raise AssertionError("webhook repository.full_name is required")
    return name


def recording_url(url: str) -> str:
    rewritten = url.replace("{api}", "")
    parsed = urlparse(rewritten)
    if parsed.scheme and parsed.netloc:
        return parsed.path + (("?" + parsed.query) if parsed.query else "")
    if rewritten.startswith("/"):
        return rewritten
    if rewritten.startswith("http"):
        parsed = urlparse(rewritten)
        return parsed.path + (("?" + parsed.query) if parsed.query else "")
    return rewritten if rewritten.startswith("/") else "/" + rewritten


class RecordedAPI:
    def __init__(self, recordings: list[dict[str, Any]]) -> None:
        self._remaining = list(recordings)
        self.calls: list[tuple[str, str]] = []

    def request(
        self,
        method: str,
        url: str,
        *,
        form: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        if not self._remaining:
            raise AssertionError(f"no recording left for {method} {url}")
        item = self._remaining.pop(0)
        self.calls.append((method, url))
        if item.get("method") != method:
            raise AssertionError(f"expected {item.get('method')} recording, got {method} {url}")
        recorded_url = recording_url(str(item.get("url", "")))
        if recorded_url != url:
            raise AssertionError(f"expected url {recorded_url}, got {url}")
        if form is not None:
            recorded_form = ((item.get("request") or {}).get("form")) or {}
            if recorded_form != form:
                raise AssertionError(f"POST form mismatch for {url}: {recorded_form} != {form}")
        return item

    def unused(self) -> list[dict[str, Any]]:
        return list(self._remaining)


def parse_link_next(headers: dict[str, Any]) -> str | None:
    link = headers.get("Link") or headers.get("link")
    if not isinstance(link, str) or not link:
        return None
    for part in link.split(","):
        piece = part.strip()
        if 'rel="next"' not in piece and "rel=next" not in piece:
            continue
        match = re.search(r"<([^>]+)>", piece)
        if match:
            return recording_url(match.group(1))
    return None


def list_open_pulls(api: RecordedAPI, repo: str) -> list[dict[str, Any]]:
    url = f"/repos/{repo}/pulls?state=open&per_page=100"
    items: list[dict[str, Any]] = []
    while url:
        rec = api.request("GET", url)
        body = rec.get("body")
        if not isinstance(body, list):
            raise AssertionError(f"{url} body must be a list")
        items.extend(body)
        url = parse_link_next(rec.get("headers") or {})
    return items


def linked_heads(pulls: list[dict[str, Any]], issue_number: int) -> list[str]:
    heads: list[str] = []
    for pull in pulls:
        body = pull.get("body") or ""
        matched = False
        for line in str(body).splitlines():
            found = LINK_RE.fullmatch(line)
            if found and int(found.group("issue")) == issue_number:
                matched = True
                break
        if not matched:
            continue
        sha = ((pull.get("head") or {}).get("sha")) or ""
        if not re.fullmatch(r"[0-9a-f]{40}", sha):
            raise AssertionError(f"linked pull {pull.get('number')} head is not 40-hex")
        heads.append(sha)
    return heads


def authorize(api: RecordedAPI, repo: str, event: dict[str, Any]) -> bool:
    association = (event.get("comment") or {}).get("author_association")
    login = ((event.get("comment") or {}).get("user") or {}).get("login")
    if association == "OWNER":
        return True
    rec = api.request("GET", f"/repos/{repo}/collaborators/{login}/permission")
    permission = (rec.get("body") or {}).get("permission")
    return permission in {"maintain", "admin"}


def revoke_form(name: str, head: str) -> dict[str, str]:
    return {
        "name": name,
        "head_sha": head,
        "status": "completed",
        "conclusion": "failure",
        "output[title]": "Intent authority changed",
        "output[summary]": "Policy checks revoked until the linked pull request is revalidated.",
    }


def replay_bundle(bundle: Path) -> dict[str, Any]:
    event = load_json(bundle / "webhook.json")
    recordings = load_json(bundle / "http" / "recordings.json")
    if not isinstance(recordings, list) or not recordings:
        raise AssertionError(f"{bundle.name} http/recordings.json must be a non-empty array")
    if not intent_job_selected("issue_comment", event):
        raise AssertionError(f"{bundle.name} would not select intent-revocation")
    repo = repository_name(event)
    api = RecordedAPI(recordings)
    if not authorize(api, repo, event):
        raise AssertionError(f"{bundle.name} authorization failed closed without expected revoke")
    issue_number = int((event.get("issue") or {}).get("number"))
    heads = linked_heads(list_open_pulls(api, repo), issue_number)
    check_runs: list[dict[str, str]] = []
    for head in heads:
        for name in CHECK_NAMES:
            rec = api.request("POST", f"/repos/{repo}/check-runs", form=revoke_form(name, head))
            body = rec.get("body") or {}
            if body.get("name") != name or body.get("conclusion") != "failure" or body.get("head_sha") != head:
                raise AssertionError(f"{bundle.name} check-run recording mismatch for {name}")
            check_runs.append({"name": name, "conclusion": "failure", "head_sha": head})
    unused = api.unused()
    if unused:
        raise AssertionError(f"{bundle.name} left unused recordings: {unused}")
    return {
        "job": "intent-revocation",
        "linked_open_pull_heads": heads,
        "check_runs": check_runs,
    }


def scan_leaks(path: Path) -> None:
    text = dump_text(path)
    if "REDACTED" not in text and path.name == "recordings.json":
        raise AssertionError(f"{path} recordings must redact Authorization")
    for pattern in LEAK_PATTERNS:
        if pattern.search(text):
            raise AssertionError(f"{path} contains a live-looking secret")


def harness_treats_as_validator_input(text: str, name: str) -> bool:
    for raw in text.splitlines():
        line = raw.strip()
        if name not in line:
            continue
        if line.startswith("#") or line.startswith("//"):
            continue
        lower = line.lower()
        if any(
            token in lower
            for token in (
                "exclude",
                "skip",
                "omit",
                "workflow-only",
                "workflow only",
                "not validator",
                "not a validator",
                "must not",
                "do not",
            )
        ):
            continue
        if "workflow" in lower and any(
            token in lower for token in ("exclude", "skip", "not", "never", "absent")
        ):
            continue
        return True
    return False


class WorkflowReplayTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.workflow = dump_text(POLICY_WORKFLOW)
        cls.jobs = job_blocks(cls.workflow)

    def test_required_bundles_exist_with_workflow_schema(self) -> None:
        for name in REQUIRED_BUNDLES:
            bundle = HERE / name
            self.assertTrue((bundle / "webhook.json").is_file(), name)
            self.assertTrue((bundle / "http" / "recordings.json").is_file(), name)
            self.assertFalse((bundle / "git").exists(), f"{name} must not carry git objects")
            expected = load_json(bundle / "expected.json")
            self.assertEqual(tuple(expected.keys()), WORKFLOW_EXPECTED_KEYS, name)
            for key in VALIDATOR_EXPECTED_KEYS:
                self.assertNotIn(key, expected, name)
            self.assertEqual(expected["job"], "intent-revocation")
            self.assertIsInstance(expected["linked_open_pull_heads"], list)
            self.assertGreaterEqual(len(expected["linked_open_pull_heads"]), 1)
            for head in expected["linked_open_pull_heads"]:
                self.assertRegex(head, r"^[0-9a-f]{40}$")
            self.assertIsInstance(expected["check_runs"], list)
            for run in expected["check_runs"]:
                self.assertEqual(tuple(run.keys()), CHECK_RUN_KEYS)
                self.assertIn(run["name"], CHECK_NAMES)
                self.assertEqual(run["conclusion"], "failure")
                self.assertIn(run["head_sha"], expected["linked_open_pull_heads"])
            webhook = load_json(bundle / "webhook.json")
            self.assertEqual(webhook["action"], name.rsplit("-", 2)[1])
            self.assertIsNone(webhook["issue"].get("pull_request"))
            self.assertEqual(webhook["issue"]["number"], 3)
            scan_leaks(bundle / "webhook.json")
            scan_leaks(bundle / "http" / "recordings.json")
            scan_leaks(bundle / "expected.json")

    def test_policy_yml_intent_revocation_gating(self) -> None:
        self.assertIn("intent-revocation", self.jobs)
        self.assertIn("publish", self.jobs)
        predicate = job_if(self.jobs["intent-revocation"])
        for needle in JOB_IF_NEEDLES:
            self.assertIn(needle, predicate)
        self.assertNotIn("pull_request_target", predicate)
        self.assertNotIn("pull_request_review", predicate)
        publish_if = job_if(self.jobs["publish"])
        self.assertIn("github.event.issue.pull_request", publish_if)
        self.assertNotIn("!github.event.issue.pull_request", publish_if)
        self.assertNotIn("repo-ops.intent.v1", publish_if)
        intent = self.jobs["intent-revocation"]
        self.assertNotIn("id: authorize", intent)
        self.assertIn('"$AUTHOR_ASSOCIATION" != "OWNER"', intent)
        self.assertIn("maintain|admin", intent)
        self.assertIn("exit 0", intent)
        self.assertIn("/pulls?state=open&per_page=100", intent)
        self.assertIn(
            '(?im)^(Fixes|Closes|Related)[[:space:]]+#" + $issue + "[[:space:]]*$',
            intent,
        )
        self.assertIn('for check_name in "contract" "merge approval"', intent)
        self.assertIn('-f "name=${check_name}"', intent)
        self.assertIn('-f "head_sha=${head_sha}"', intent)
        self.assertIn('-f "conclusion=failure"', intent)
        self.assertIn('-f "status=completed"', intent)
        self.assertIn("Intent authority changed", intent)
        self.assertIn("  intent-revocation:", self.workflow)
        self.assertNotIn("\n    name: contract\n", self.workflow)
        self.assertNotIn("\n    name: merge approval\n", self.workflow)

    def test_policy_job_permissions_are_read_read_read_write(self) -> None:
        self.assertRegex(self.workflow, r"(?m)^permissions: \{\}\s*$")
        for job_name in ("publish", "intent-revocation"):
            parsed = job_permissions(self.jobs[job_name])
            self.assertEqual(parsed, JOB_PERMISSIONS, job_name)

    def test_owner_skips_permission_lookup(self) -> None:
        recordings = load_json(
            HERE / "issue_comment-created-intent" / "http" / "recordings.json"
        )
        webhook = load_json(HERE / "issue_comment-created-intent" / "webhook.json")
        self.assertEqual(webhook["comment"]["author_association"], "OWNER")
        for item in recordings:
            self.assertNotIn("/collaborators/", item["url"])

    def test_member_and_collaborator_use_permission_endpoint(self) -> None:
        edited = load_json(HERE / "issue_comment-edited-intent" / "http" / "recordings.json")
        deleted = load_json(HERE / "issue_comment-deleted-intent" / "http" / "recordings.json")
        self.assertEqual(edited[0]["url"], "/repos/haesol-shin/.github/collaborators/reviewer/permission")
        self.assertEqual((edited[0]["body"] or {}).get("permission"), "maintain")
        self.assertEqual(deleted[0]["url"], "/repos/haesol-shin/.github/collaborators/helper/permission")
        self.assertEqual((deleted[0]["body"] or {}).get("permission"), "admin")
        edited_hook = load_json(HERE / "issue_comment-edited-intent" / "webhook.json")
        deleted_hook = load_json(HERE / "issue_comment-deleted-intent" / "webhook.json")
        self.assertEqual(edited_hook["comment"]["author_association"], "MEMBER")
        self.assertEqual(deleted_hook["comment"]["author_association"], "COLLABORATOR")
        self.assertIn("repo-ops.intent.v1", edited_hook["comment"]["body"])
        self.assertIn("Intent accepted.", edited_hook["changes"]["body"]["from"])
        self.assertIn("Intent accepted.", deleted_hook["comment"]["body"])

    def test_unauthorized_permission_publishes_no_checks(self) -> None:
        event = load_json(HERE / "issue_comment-edited-intent" / "webhook.json")
        recordings = [
            {
                "method": "GET",
                "url": "/repos/haesol-shin/.github/collaborators/reviewer/permission",
                "status": 200,
                "headers": {},
                "body": {"permission": "write", "role_name": "write"},
            }
        ]
        api = RecordedAPI(recordings)
        self.assertTrue(intent_job_selected("issue_comment", event))
        self.assertFalse(authorize(api, repository_name(event), event))
        self.assertEqual(api.unused(), [])

    def test_gating_rejects_pr_comments_and_untrusted_authors(self) -> None:
        base = load_json(HERE / "issue_comment-created-intent" / "webhook.json")
        pr_comment = json.loads(json.dumps(base))
        pr_comment["issue"]["pull_request"] = {"url": "https://api.github.com/repos/haesol-shin/.github/pulls/6"}
        self.assertFalse(intent_job_selected("issue_comment", pr_comment))
        none = json.loads(json.dumps(base))
        none["comment"]["author_association"] = "NONE"
        self.assertFalse(intent_job_selected("issue_comment", none))
        empty = json.loads(json.dumps(base))
        empty["comment"]["body"] = "ordinary issue comment"
        self.assertFalse(intent_job_selected("issue_comment", empty))
        self.assertFalse(intent_job_selected("pull_request_target", base))

    def test_replay_matches_committed_expected(self) -> None:
        for name in REQUIRED_BUNDLES:
            observed = replay_bundle(HERE / name)
            expected = load_json(HERE / name / "expected.json")
            self.assertEqual(observed, expected, name)
            names = [run["name"] for run in observed["check_runs"]]
            self.assertEqual(set(names), set(CHECK_NAMES))
            self.assertEqual(names.count("contract"), len(observed["linked_open_pull_heads"]))
            self.assertEqual(names.count("merge approval"), len(observed["linked_open_pull_heads"]))

    def test_unlinked_open_heads_are_not_revoked(self) -> None:
        recordings = load_json(HERE / "issue_comment-created-intent" / "http" / "recordings.json")
        pulls = recordings[0]["body"]
        self.assertEqual(len(pulls), 2)
        heads = linked_heads(pulls, 3)
        self.assertEqual(heads, ["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"])
        self.assertIn("cccccccccccccccccccccccccccccccccccccccc", [pull["head"]["sha"] for pull in pulls])

    def test_bundles_are_excluded_from_validator_replay(self) -> None:
        self.assertTrue(COMPARE.is_file())
        compare_text = dump_text(COMPARE)
        self.assertNotIn("events/workflow", compare_text)
        self.assertIn("intent-comment-revocation.json", compare_text)
        for name in REQUIRED_BUNDLES:
            self.assertFalse(harness_treats_as_validator_input(compare_text, name), name)
        if VALIDATOR_REPLAY.is_file():
            replay_text = dump_text(VALIDATOR_REPLAY)
            for name in REQUIRED_BUNDLES:
                self.assertFalse(
                    harness_treats_as_validator_input(replay_text, name),
                    f"validator replay harness treats {name} as CLI input",
                )
            for line in replay_text.replace("\\", "/").splitlines():
                if "events/workflow" not in line:
                    continue
                lower = line.lower()
                self.assertTrue(
                    any(
                        token in lower
                        for token in (
                            "exclude",
                            "skip",
                            "does not",
                            "do not",
                            "never",
                            "workflow-only",
                            "not load",
                            "omit",
                        )
                    ),
                    f"events/replay.py mentions workflow without exclusion: {line}",
                )


if __name__ == "__main__":
    sys.exit(0 if unittest.main(verbosity=2, exit=False).result.wasSuccessful() else 1)
