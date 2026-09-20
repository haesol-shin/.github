from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

import yaml
from jsonschema import Draft202012Validator

CONTRACT_ROOT = Path(__file__).resolve().parents[2] / "contracts" / "v0.1.0"
CONTRACT = json.loads((CONTRACT_ROOT / "contract.json").read_text(encoding="utf-8"))
CONTRACT_VERSION = CONTRACT["id"]
ROUND_LIMITS = CONTRACT["round_limits"]
RECORD_ORDERS = {
    record: tuple(definition["field_order"])
    for record, definition in CONTRACT["records"].items()
}
AUTHORIZED_ASSOCIATIONS = {"OWNER", "MEMBER"}


def normalize_text(value: str) -> str:
    normalized = value.replace("\r\n", "\n").replace("\r", "\n").rstrip()
    return f"{normalized}\n"


def markdown_headings(body: str, level: int) -> list[str]:
    prefix = "#" * level
    return [
        line.rstrip()
        for line in body.splitlines()
        if re.fullmatch(rf"{re.escape(prefix)}(?!#)[ \t]+\S.*", line.rstrip())
    ]


def validate_heading_contract(
    body: str,
    definition: dict[str, Any],
    label: str,
) -> list[str]:
    headings = markdown_headings(body, int(definition["heading_level"]))
    required = definition["required_headings"]
    required_occurrences = int(definition["required_heading_occurrences"])
    errors: list[str] = []

    for heading in required:
        count = headings.count(heading)
        if count < required_occurrences:
            errors.append(f"{label} is missing {heading}")
        elif count > required_occurrences:
            errors.append(f"{label} contains duplicate {heading}")

    if definition.get("required_heading_order", False) and all(
        headings.count(heading) == required_occurrences for heading in required
    ):
        positions = [headings.index(heading) for heading in required]
        if positions != sorted(positions):
            errors.append(f"{label} required headings must appear in contract order")

    if not definition.get("allow_additional_headings", False):
        extras = [heading for heading in headings if heading not in required]
        if extras:
            errors.append(f"{label} contains unsupported headings: {', '.join(extras)}")

    return errors


def canonical_digest(payload: dict[str, Any]) -> str:
    encoded = json.dumps(
        payload,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")
    return f"sha256:{hashlib.sha256(encoded).hexdigest()}"


def plan_digest(content: str) -> str:
    return canonical_digest({"text": normalize_text(content)})


def parse_record_line(body: str, record: str) -> tuple[dict[str, Any] | None, str | None]:
    matching = [line.strip() for line in body.splitlines() if line.strip().startswith(record)]
    if not matching:
        return None, None
    if len(matching) > 1:
        return None, f"{record} must appear exactly once in one comment"

    tokens = matching[0].split(" ")
    if any(not token for token in tokens):
        return None, f"{record} fields must use one ASCII space"
    if tokens[0] != record:
        return None, f"malformed {record} identifier"

    values: dict[str, Any] = {"record": record}
    keys: list[str] = []
    for token in tokens[1:]:
        if ":" not in token:
            return None, f"malformed {record} field: {token}"
        key, value = token.split(":", 1)
        if not key or not value or key in values:
            return None, f"malformed or duplicate {record} field: {token}"
        keys.append(key)
        values[key] = int(value) if key == "issue" and value.isdigit() else value

    if tuple(keys) != RECORD_ORDERS[record]:
        expected = " ".join(RECORD_ORDERS[record])
        return None, f"{record} fields must appear in this order: {expected}"
    return values, None


def validate_schema(instance: Any, schema_path: Path, label: str) -> list[str]:
    schema = json.loads(schema_path.read_text(encoding="utf-8"))
    errors = sorted(Draft202012Validator(schema).iter_errors(instance), key=lambda error: list(error.path))
    return [f"{label}: {error.message}" for error in errors]


def parse_risk(body: str) -> str | None:
    match = re.search(r"(?m)^Risk:\s*(low|medium|high)\s+(?:—|-)\s+\S", body)
    return match.group(1) if match else None


def parse_related_issue(body: str) -> int | None:
    match = re.search(r"(?mi)^\s*(?:Fixes|Closes)\s+#(\d+)\s*$", body)
    return int(match.group(1)) if match else None


def is_direct_low_risk(body: str) -> bool:
    return bool(re.search(r"(?mi)^\s*Direct low-risk PR:\s*\S", body))


def parse_plan_path(body: str) -> str | None:
    match = re.search(r"(?mi)^Plan:\s*(.+?)\s*$", body)
    if not match:
        return None
    value = match.group(1).strip().strip("`")
    if value.lower() == "none":
        return None
    markdown_link = re.fullmatch(r"\[[^]]+\]\(([^)]+)\)", value)
    if markdown_link:
        value = markdown_link.group(1)
    if value.startswith("http://") or value.startswith("https://"):
        url_path = urllib.parse.urlparse(value).path
        blob_match = re.search(r"/blob/[^/]+/(.+)$", url_path)
        if not blob_match:
            return None
        value = blob_match.group(1)
    if value.startswith("./"):
        value = value[2:]
    return value if value.startswith(".ops/plans/") else None


def parse_intent(body: str) -> tuple[dict[str, str] | None, str | None]:
    normalized = normalize_text(body)
    match = re.fullmatch(
        r"Intent accepted\.\n\n(?P<outcome>.+?)\n\nRisk: (?P<risk>low|medium|high)\nIntent digest: (?P<digest>sha256:[0-9a-f]{64})\n",
        normalized,
        flags=re.DOTALL,
    )
    if not match:
        return None, None
    computed = canonical_digest(
        {
            "outcome": normalize_text(match.group("outcome")),
            "risk": match.group("risk"),
        }
    )
    if computed != match.group("digest"):
        return None, "accepted-intent digest does not match its canonical payload"
    return {"digest": computed, "risk": match.group("risk")}, None


def parse_round(value: str, *, allow_none: bool) -> tuple[int | None, int | None]:
    if allow_none and value == "none":
        return None, None
    match = re.fullmatch(r"([1-9][0-9]*)/([1-9][0-9]*)", value)
    if not match:
        return None, None
    return int(match.group(1)), int(match.group(2))


def latest_record(
    entries: list[dict[str, Any]],
    record: str,
) -> tuple[dict[str, Any] | None, dict[str, Any] | None, list[str]]:
    found: list[tuple[dict[str, Any], dict[str, Any]]] = []
    errors: list[str] = []
    for entry in entries:
        parsed, error = parse_record_line(entry.get("body") or "", record)
        if error:
            errors.append(error)
        elif parsed:
            found.append((parsed, entry))
    if not found:
        return None, None, errors
    parsed, entry = found[-1]
    return parsed, entry, errors


def validate_state(state: dict[str, Any], contract_root: Path) -> list[str]:
    errors: list[str] = []
    policy = state.get("policy")
    if not isinstance(policy, dict):
        return ["repository policy is missing or invalid YAML"]

    errors.extend(
        validate_schema(
            policy,
            contract_root / "repository-policy.schema.json",
            "repository policy",
        )
    )
    if policy.get("contract") != CONTRACT_VERSION:
        errors.append(f"repository policy must adopt {CONTRACT_VERSION}")

    pull_request = state.get("pull_request") or {}
    body = pull_request.get("body") or ""
    errors.extend(
        validate_heading_contract(body, CONTRACT["pull_request"], "pull request body")
    )

    risk = parse_risk(body)
    if risk is None:
        errors.append("pull request body must contain `Risk: low|medium|high — rationale`")
        return errors

    issue = state.get("issue")
    direct = is_direct_low_risk(body)
    if direct and risk != "low":
        errors.append("only low-risk work may use the direct pull request route")
    if not direct and issue is None:
        errors.append("non-direct work must link an issue with `Fixes #<issue>`")
    comments = state.get("comments") or []
    if issue is not None:
        issue_body = state.get("issue_body") or ""
        errors.extend(
            validate_heading_contract(issue_body, CONTRACT["issue"], "linked issue body")
        )
    reviews = state.get("reviews") or []
    authorized_comments = [
        entry for entry in comments if entry.get("association") in AUTHORIZED_ASSOCIATIONS
    ]
    authorized_reviews = [
        entry for entry in reviews if entry.get("association") in AUTHORIZED_ASSOCIATIONS
    ]
    records = [
        *({**entry, "surface": "comment"} for entry in authorized_comments),
        *({**entry, "surface": "review"} for entry in authorized_reviews),
    ]

    expected_intent = "none"
    if issue is not None:
        intents: list[dict[str, str]] = []
        for comment in state.get("intent_comments") or []:
            if comment.get("association") not in AUTHORIZED_ASSOCIATIONS:
                continue
            parsed_intent, intent_error = parse_intent(comment.get("body") or "")
            if intent_error:
                errors.append(intent_error)
            elif parsed_intent:
                intents.append(parsed_intent)
        if not intents:
            errors.append("linked issue has no valid maintainer accepted-intent comment")
        else:
            expected_intent = intents[-1]["digest"]
            if intents[-1]["risk"] != risk:
                errors.append("pull request risk does not match accepted intent")

    expected_plan = state.get("plan_digest") or "none"
    if risk in {"medium", "high"} and expected_plan == "none":
        errors.append(f"{risk}-risk work requires a current plan")
    if risk == "low" and expected_plan != "none":
        errors.append("low-risk direct work must not claim a plan digest")

    receipt, receipt_entry, receipt_errors = latest_record(records, "repo-ops.merge-review.v1")
    errors.extend(receipt_errors)
    if receipt is None:
        errors.append("no merge-ready receipt was found")
    else:
        errors.extend(
            validate_schema(
                receipt,
                contract_root / "merge-review.schema.json",
                "merge-review receipt",
            )
        )
        expected = {
            "risk": risk,
            "intent": expected_intent,
            "plan": expected_plan,
            "base": pull_request.get("merge_base"),
            "head": pull_request.get("head"),
            "diff": pull_request.get("diff_digest"),
        }
        for key, value in expected.items():
            if value is not None and receipt.get(key) != value:
                errors.append(f"merge-review {key} does not match the current pull request")

        plan_round, plan_max = parse_round(receipt.get("plan-round", ""), allow_none=True)
        implementation_round, implementation_max = parse_round(
            receipt.get("implementation-round", ""),
            allow_none=False,
        )
        limits = ROUND_LIMITS[risk]
        if limits["plan"] is None:
            if (plan_round, plan_max) != (None, None):
                errors.append("low-risk merge review must use plan-round:none")
        elif plan_max != limits["plan"] or plan_round is None or plan_round > plan_max:
            errors.append(f"{risk}-risk plan round must be n/{limits['plan']}")
        if (
            implementation_max != limits["implementation"]
            or implementation_round is None
            or implementation_round > implementation_max
        ):
            errors.append(
                f"{risk}-risk implementation round must be n/{limits['implementation']}"
            )

        if receipt_entry and receipt_entry.get("association") not in AUTHORIZED_ASSOCIATIONS:
            errors.append("merge review was not posted by an authorized maintainer")
        if policy.get("review", {}).get("shared_identity") is False:
            if receipt_entry and receipt_entry.get("surface") != "review":
                errors.append("independent review receipt must be submitted as a native GitHub Review")
            if receipt_entry and receipt_entry.get("author") == pull_request.get("author"):
                errors.append("independent review requires an identity distinct from the pull request author")

    if risk == "high":
        approval, approval_entry, approval_errors = latest_record(
            authorized_comments,
            "repo-ops.plan-approval.v1",
        )
        errors.extend(approval_errors)
        if approval is None:
            errors.append("high-risk work requires a maintainer plan approval")
        else:
            errors.extend(
                validate_schema(
                    approval,
                    contract_root / "plan-approval.schema.json",
                    "plan approval",
                )
            )
            expected_approval = {
                "issue": issue,
                "intent": expected_intent,
                "plan": expected_plan,
            }
            for key, value in expected_approval.items():
                if value is not None and approval.get(key) != value:
                    errors.append(f"plan approval {key} does not match current work")
            if approval_entry and approval_entry.get("association") not in AUTHORIZED_ASSOCIATIONS:
                errors.append("plan approval was not posted by an authorized maintainer")
            if approval.get("plan-commit") not in (state.get("plan_commits") or []):
                errors.append("approved plan commit is not in the pull request history")

    quality = state.get("quality") or {}
    expected_quality_name = policy.get("quality", {}).get("check")
    if receipt is not None and (
        quality.get("name") != expected_quality_name
        or quality.get("conclusion") != "success"
    ):
        errors.append(f"{expected_quality_name} has not succeeded for the current head")

    return errors


class GitHubClient:
    def __init__(self, token: str, api_url: str = "https://api.github.com") -> None:
        self.token = token
        self.api_url = api_url.rstrip("/")

    def _request(
        self,
        url_or_path: str,
        *,
        accept: str,
    ) -> tuple[Any, str | None]:
        url = url_or_path if url_or_path.startswith("http") else f"{self.api_url}{url_or_path}"
        request = urllib.request.Request(
            url,
            headers={
                "Accept": accept,
                "Authorization": f"Bearer {self.token}",
                "X-GitHub-Api-Version": "2022-11-28",
                "User-Agent": "repo-ops-validator/0.1",
            },
        )
        try:
            with urllib.request.urlopen(request, timeout=30) as response:
                content = response.read()
                link = response.headers.get("Link")
        except urllib.error.HTTPError as error:
            detail = error.read().decode("utf-8", errors="replace")
            raise RuntimeError(f"GitHub API {error.code} for {url}: {detail}") from error
        return json.loads(content), link

    def get(self, url_or_path: str, *, accept: str = "application/vnd.github+json") -> Any:
        data, _ = self._request(url_or_path, accept=accept)
        return data

    def get_all(
        self,
        url_or_path: str,
        *,
        field: str | None = None,
        accept: str = "application/vnd.github+json",
    ) -> list[Any]:
        url: str | None = url_or_path
        items: list[Any] = []
        while url:
            data, link = self._request(url, accept=accept)
            page = data.get(field, []) if field else data
            if not isinstance(page, list):
                raise RuntimeError(f"paginated GitHub API response is not a list: {url}")
            items.extend(page)
            next_match = re.search(r'<([^>]+)>; rel="next"', link or "")
            url = next_match.group(1) if next_match else None
        return items


def api_content(client: GitHubClient, repository: str, path: str, ref: str) -> str:
    encoded_path = urllib.parse.quote(path, safe="/")
    result = client.get(f"/repos/{repository}/contents/{encoded_path}?ref={urllib.parse.quote(ref)}")
    if result.get("encoding") != "base64":
        raise RuntimeError(f"unsupported content encoding for {path}")
    return base64.b64decode(result["content"]).decode("utf-8")


def git_metadata(
    pull_request: dict[str, Any],
    token: str,
    merge_base: str,
) -> tuple[str, str]:
    number = pull_request["number"]
    head_sha = pull_request["head"]["sha"]
    base_url = pull_request["base"]["repo"]["clone_url"]

    with tempfile.TemporaryDirectory(prefix="repo-ops-") as directory:
        environment = os.environ.copy()
        if token:
            basic = base64.b64encode(f"x-access-token:{token}".encode()).decode()
            environment.update(
                {
                    "GIT_CONFIG_COUNT": "1",
                    "GIT_CONFIG_KEY_0": "http.https://github.com/.extraheader",
                    "GIT_CONFIG_VALUE_0": f"AUTHORIZATION: basic {basic}",
                }
            )
        subprocess.run(["git", "init", "--quiet", directory], check=True, env=environment)
        subprocess.run(
            [
                "git",
                "fetch",
                "--quiet",
                "--no-tags",
                base_url,
                f"refs/pull/{number}/head:refs/repo-ops/head",
            ],
            cwd=directory,
            check=True,
            env=environment,
        )
        fetched_head = subprocess.check_output(
            ["git", "rev-parse", "refs/repo-ops/head"],
            cwd=directory,
            env=environment,
        ).decode().strip()
        if fetched_head != head_sha:
            raise RuntimeError("fetched pull request head does not match the GitHub API")
        subprocess.run(
            ["git", "cat-file", "-e", f"{merge_base}^{{commit}}"],
            cwd=directory,
            check=True,
            env=environment,
        )
        subprocess.run(
            ["git", "update-ref", "refs/repo-ops/base", merge_base],
            cwd=directory,
            check=True,
            env=environment,
        )

        command = [
            "git",
            "-c",
            "core.quotePath=true",
            "diff",
            "--binary",
            "--full-index",
            "--no-color",
            "--no-ext-diff",
            "--no-textconv",
            "--no-renames",
            "refs/repo-ops/base",
            "refs/repo-ops/head",
            "--",
        ]
        process = subprocess.Popen(
            command,
            cwd=directory,
            env=environment,
            stdout=subprocess.PIPE,
        )
        digest = hashlib.sha256()
        assert process.stdout is not None
        while chunk := process.stdout.read(1024 * 1024):
            digest.update(chunk)
        return_code = process.wait()
        if return_code:
            raise subprocess.CalledProcessError(return_code, command)
    return merge_base, f"sha256:{digest.hexdigest()}"


def entry_from_api(item: dict[str, Any]) -> dict[str, Any]:
    return {
        "body": item.get("body") or "",
        "author": (item.get("user") or {}).get("login"),
        "association": item.get("author_association"),
    }


def build_live_state(event_path: Path) -> dict[str, Any]:
    event = json.loads(event_path.read_text(encoding="utf-8"))
    token = os.environ.get("GITHUB_TOKEN", "")
    if not token:
        raise RuntimeError("GITHUB_TOKEN is required for live validation")
    client = GitHubClient(token, os.environ.get("GITHUB_API_URL", "https://api.github.com"))

    repository = (event.get("repository") or {}).get("full_name") or os.environ["GITHUB_REPOSITORY"]
    pull_request = event.get("pull_request")
    if pull_request is None and (event.get("issue") or {}).get("pull_request"):
        pull_request = client.get(event["issue"]["pull_request"]["url"])
    if pull_request is None:
        raise RuntimeError("event is not associated with a pull request")

    number = pull_request["number"]
    base_sha = pull_request["base"]["sha"]
    body = pull_request.get("body") or ""
    policy_text = api_content(client, repository, ".github/repo-policy.yml", base_sha)
    policy = yaml.safe_load(policy_text)
    comparison = client.get(
        f"/repos/{repository}/compare/{base_sha}...{pull_request['head']['sha']}"
    )
    merge_base_sha = comparison["merge_base_commit"]["sha"]
    merge_base, diff_digest = git_metadata(pull_request, token, merge_base_sha)

    pr_comments = [
        entry_from_api(item)
        for item in client.get_all(
            f"/repos/{repository}/issues/{number}/comments?per_page=100"
        )
    ]
    reviews = [
        entry_from_api(item)
        for item in client.get_all(
            f"/repos/{repository}/pulls/{number}/reviews?per_page=100"
        )
        if item.get("body") and item.get("state") != "DISMISSED"
    ]

    issue_number = parse_related_issue(body)
    intent_comments: list[dict[str, Any]] = []
    issue_body = None
    if issue_number is not None:
        issue_document = client.get(f"/repos/{repository}/issues/{issue_number}")
        issue_body = issue_document.get("body") or ""
        intent_comments = [
            entry_from_api(item)
            for item in client.get_all(
                f"/repos/{repository}/issues/{issue_number}/comments?per_page=100"
            )
        ]

    plan_path = parse_plan_path(body)
    current_plan_digest = "none"
    if plan_path:
        current_plan_digest = plan_digest(
            api_content(client, repository, plan_path, pull_request["head"]["sha"])
        )

    approval_commits: list[str] = []
    for comment in (
        entry
        for entry in pr_comments
        if entry.get("association") in AUTHORIZED_ASSOCIATIONS
    ):
        approval, _ = parse_record_line(comment["body"], "repo-ops.plan-approval.v1")
        if not approval:
            continue
        plan_commit = approval["plan-commit"]
        comparison = client.get(
            f"/repos/{repository}/compare/{plan_commit}...{pull_request['head']['sha']}"
        )
        approved_plan_digest = None
        if plan_path:
            approved_plan_digest = plan_digest(
                api_content(client, repository, plan_path, plan_commit)
            )
        if (
            comparison.get("status") in {"ahead", "identical"}
            and approved_plan_digest == approval.get("plan")
            and approved_plan_digest == current_plan_digest
        ):
            approval_commits.append(plan_commit)

    check_runs = client.get_all(
        f"/repos/{repository}/commits/{pull_request['head']['sha']}/check-runs?per_page=100",
        field="check_runs",
        accept="application/vnd.github+json",
    )
    quality_name = (policy or {}).get("quality", {}).get("check")
    matching_quality = [run for run in check_runs if run.get("name") == quality_name]
    quality = {
        "name": quality_name,
        "conclusion": matching_quality[-1].get("conclusion") if matching_quality else None,
    }

    return {
        "policy": policy,
        "pull_request": {
            "number": number,
            "author": (pull_request.get("user") or {}).get("login"),
            "body": body,
            "base": pull_request["base"]["sha"],
            "merge_base": merge_base,
            "head": pull_request["head"]["sha"],
            "diff_digest": diff_digest,
        },
        "issue": issue_number,
        "issue_body": issue_body,
        "intent_comments": intent_comments,
        "comments": pr_comments,
        "reviews": reviews,
        "plan_digest": current_plan_digest,
        "plan_commits": approval_commits,
        "quality": quality,
    }


def load_fixture(path: Path) -> tuple[dict[str, Any], str | None]:
    document = json.loads(path.read_text(encoding="utf-8"))
    if "extends" not in document:
        return document, document.get("expected")

    state, _ = load_fixture(path.parent / document["extends"])
    state = json.loads(json.dumps(state))
    for dotted_path, value in (document.get("replace") or {}).items():
        target: Any = state
        parts = dotted_path.split(".")
        for part in parts[:-1]:
            target = target[int(part)] if isinstance(target, list) else target[part]
        final = parts[-1]
        if isinstance(target, list):
            target[int(final)] = value
        else:
            target[final] = value
    return state, document.get("expected")


def emit_findings(findings: list[str], mode: str) -> None:
    if findings:
        for finding in findings:
            print(f"::warning title=Repository policy::{finding}")
        summary = os.environ.get("GITHUB_STEP_SUMMARY")
        if summary:
            with open(summary, "a", encoding="utf-8") as stream:
                stream.write("## Repository policy findings\n\n")
                for finding in findings:
                    stream.write(f"- {finding}\n")
        print(f"Repository policy: {len(findings)} finding(s) in {mode} mode")
    else:
        print("Repository policy: contract satisfied")


def main() -> int:
    parser = argparse.ArgumentParser()
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--event", type=Path)
    source.add_argument("--fixture", type=Path)
    parser.add_argument("--mode", choices=("advisory", "enforce"), default="advisory")
    args = parser.parse_args()

    repository_root = Path(__file__).resolve().parents[2]
    contract_root = repository_root / "contracts" / "v0.1.0"
    try:
        if args.fixture:
            state, _ = load_fixture(args.fixture)
        else:
            state = build_live_state(args.event)
        findings = validate_state(state, contract_root)
    except (OSError, RuntimeError, ValueError, yaml.YAMLError) as error:
        findings = [str(error)]

    emit_findings(findings, args.mode)
    return 1 if findings and args.mode == "enforce" else 0


if __name__ == "__main__":
    sys.exit(main())
