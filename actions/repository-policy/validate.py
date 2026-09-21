from __future__ import annotations

import argparse
import base64
from collections.abc import Iterator
import hashlib
import importlib.util
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

RECORD_ORDERS = {
    record: tuple(definition["field_order"])
    for record, definition in CONTRACT["records"].items()
}
AUTHORITY_LINKS = CONTRACT["pull_request"].get("authority_links", {})

FRAGMENT_FILENAME_PATTERN = re.compile(
    r"^(?P<issue>[1-9][0-9]*|direct)-(?P<slug>[a-z0-9]+(?:-[a-z0-9]+)*)\.md$"
)


def parse_changelog_declaration(
    body: str,
    contract_root: Path | None = None,
) -> tuple[dict[str, str] | None, list[str]]:
    record = "repo-ops.changelog.v1"
    candidates: list[str] = []
    for line in unfenced_lines(body):
        stripped = line.strip()
        if not stripped:
            continue
        if stripped.split()[0] == record:
            candidates.append(stripped)
    if len(candidates) != 1:
        return None, [
            "pull request body must contain exactly one repo-ops.changelog.v1 record"
        ]
    parsed, error = parse_record_line(candidates[0], record)
    if parsed is None or error:
        return None, ["malformed repo-ops.changelog.v1 record"]
    schema_errors = validate_schema(
        parsed,
        (contract_root or CONTRACT_ROOT) / CONTRACT["records"][record]["schema"],
        "changelog declaration",
    )
    if schema_errors:
        return None, schema_errors
    return {"kind": str(parsed["kind"]), "value": str(parsed["value"])}, []


def _changelog_helpers() -> Any:
    try:
        from changelog import parse_fragment
    except ModuleNotFoundError:
        path = Path(__file__).with_name("changelog.py")
        spec = importlib.util.spec_from_file_location("repo_ops_changelog", path)
        if spec is None or spec.loader is None:
            raise RuntimeError("repository changelog helper is unavailable")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        parse_fragment = module.parse_fragment
    return parse_fragment


def _state_files(state: dict[str, Any]) -> list[dict[str, Any]]:
    pull_request = state.get("pull_request") or {}
    files = state.get("files", pull_request.get("files", []))
    return [entry for entry in files if isinstance(entry, dict) and isinstance(entry.get("filename"), str)]


def _fragment_path(filename: str, root: str) -> bool:
    normalized = filename.replace("\\", "/")
    prefix = root.strip("/").replace("\\", "/") + "/"
    relative = normalized[len(prefix) :] if normalized.startswith(prefix) else None
    return (
        relative is not None
        and "/" not in relative
        and relative.endswith(".md")
        and relative != "README.md"
    )


def _fragment_name_valid(filename: str, root: str) -> bool:
    if not _fragment_path(filename, root):
        return False
    return FRAGMENT_FILENAME_PATTERN.fullmatch(filename.rsplit("/", 1)[-1]) is not None


def _fragment_content(state: dict[str, Any], entry: dict[str, Any]) -> str | None:
    filename = entry["filename"]
    for key in ("file_contents", "fragment_contents"):
        contents = state.get(key)
        if isinstance(contents, dict) and isinstance(contents.get(filename), str):
            return contents[filename]
    if isinstance(entry.get("content"), str):
        return entry["content"]
    return None


def validate_changelog_state(
    state: dict[str, Any],
    *,
    policy: dict[str, Any],
    issue: int | None,
    direct: bool,
    declaration: dict[str, str] | None,
) -> list[str]:
    errors: list[str] = []
    files = _state_files(state)
    changelog_policy = policy.get("changelog") or {}
    mode = changelog_policy.get("mode")
    root = changelog_policy.get("root")
    if mode != "fragments":
        root = None

    if declaration is None:
        return errors
    kind = declaration["kind"]
    if kind in {"required", "release"} and mode != "fragments":
        errors.append("changelog fragments are required but repository policy does not enable fragments")
        return errors
    if kind == "release" and not (policy.get("release") or {}).get("enabled", False):
        errors.append("release changelog declarations require release folding to be enabled")
    if not root:
        root = "changelog.d"

    fragment_entries = [entry for entry in files if _fragment_path(entry["filename"], root)]
    changed_fragments = [
        entry
        for entry in fragment_entries
        if str(entry.get("status", "")).lower() in {"added", "modified"}
    ]
    deleted_fragments = [
        entry
        for entry in fragment_entries
        if str(entry.get("status", "")).lower() == "removed"
    ]
    direct_changelog = [
        entry
        for entry in files
        if entry["filename"].replace("\\", "/") == "CHANGELOG.md"
    ]
    release_changelog = [
        entry
        for entry in direct_changelog
        if str(entry.get("status", "")).lower() in {"added", "modified"}
    ]

    if kind != "release" and direct_changelog:
        errors.append("ordinary pull requests must not edit CHANGELOG.md")
    if kind != "release" and deleted_fragments:
        errors.append("fragment deletion is reserved for release changelog declarations")
    if kind == "required" and not changed_fragments:
        errors.append("required changelog declarations need an added or modified fragment")
    if kind == "release":
        if not release_changelog:
            errors.append("release changelog declarations must update CHANGELOG.md")
        if not deleted_fragments:
            errors.append("release changelog declarations must consume at least one fragment")
        if changed_fragments:
            errors.append("release changelog declarations must consume, not modify, fragments")

    for entry in [*changed_fragments, *deleted_fragments]:
        filename = entry["filename"]
        filename_match = FRAGMENT_FILENAME_PATTERN.fullmatch(filename.rsplit("/", 1)[-1])
        if kind != "release":
            if filename_match is None:
                errors.append("fragment filename must use <issue>-<slug>.md or direct-<slug>.md")
            elif direct:
                if not filename_match.group("issue") == "direct":
                    errors.append("direct low-risk fragments must use direct-<slug>.md")
            elif issue is None:
                errors.append("issue-backed fragments require a linked issue")
            elif filename_match.group("issue") != str(issue):
                errors.append("issue-backed fragment filename must begin with the linked issue number")
        elif not _fragment_name_valid(filename, root):
            errors.append("release fragment consumption includes an invalid fragment filename")
    parse_fragment = _changelog_helpers()
    for entry in changed_fragments:
        content = _fragment_content(state, entry)
        if content is None:
            errors.append(f"fragment content is unavailable for {entry['filename']}")
            continue
        try:
            parse_fragment(content, filename=entry["filename"])
        except ValueError as error:
            errors.append(str(error))
    return errors
AUTHORIZED_ASSOCIATIONS = {"OWNER"}
REPOSITORY_PERMISSIONS = {"maintain", "admin"}
RISK_AUTHORITY = CONTRACT.get(
    "risk_authority",
    {
        "low": {"approval_associations": [], "approval_permissions": []},
        "medium": {
            "approval_associations": ["OWNER"],
            "approval_permissions": ["maintain", "admin"],
        },
        "high": {"approval_associations": ["OWNER"], "approval_permissions": []},
    },
)


def record_candidate(entry: dict[str, Any]) -> bool:
    return (
        entry.get("association") in AUTHORIZED_ASSOCIATIONS
        or entry.get("permission") in REPOSITORY_PERMISSIONS
    )


def record_authorized(
    entry: dict[str, Any],
    *,
    record: str,
    risk: str | None = None,
) -> bool:
    if record == "repo-ops.plan-approval.v1":
        if risk is None:
            return False
        authority = RISK_AUTHORITY.get(risk, {})
        return (
            entry.get("association") in authority.get("approval_associations", [])
            or entry.get("permission") in authority.get("approval_permissions", [])
        )
    return (
        entry.get("association") == "OWNER"
        or entry.get("permission") in REPOSITORY_PERMISSIONS
    )


def normalize_text(value: str) -> str:
    normalized = value.replace("\r\n", "\n").replace("\r", "\n").rstrip()
    return f"{normalized}\n"


def unfenced_lines(body: str) -> Iterator[str]:
    fence_character: str | None = None
    fence_length = 0
    for line in re.split(r"\r\n?|\n", body):
        fence_match = re.match(r" {0,3}(?P<marker>`{3,}|~{3,})(?P<rest>.*)$", line)
        if fence_character is not None:
            if fence_match:
                marker = fence_match.group("marker")
                if (
                    marker[0] == fence_character
                    and len(marker) >= fence_length
                    and re.fullmatch(r"[ \t]*", fence_match.group("rest"))
                ):
                    fence_character = None
                    fence_length = 0
            continue
        if fence_match:
            marker = fence_match.group("marker")
            if marker[0] == "`" and "`" in fence_match.group("rest"):
                yield line
                continue
            fence_character = marker[0]
            fence_length = len(marker)
            continue
        yield line


def markdown_block_lines(body: str) -> Iterator[str]:
    for line in unfenced_lines(body):
        if re.match(r" {0,3}\S", line):
            yield line


def markdown_headings(body: str, level: int) -> list[str]:
    prefix = "#" * level
    headings: list[str] = []
    for line in markdown_block_lines(body):
        heading_match = re.fullmatch(
            rf" {{0,3}}{re.escape(prefix)}(?!#)[ \t]+(?P<title>\S.*?)[ \t]*",
            line,
        )
        if heading_match:
            headings.append(f"{prefix} {heading_match.group('title')}")

    return headings


def validate_heading_contract(
    body: str,
    definition: dict[str, Any],
    label: str,
) -> list[str]:
    del body, definition, label
    return []


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


def parse_authority_links(body: str) -> tuple[list[dict[str, Any]], list[str]]:
    keywords = tuple(AUTHORITY_LINKS.get("keywords", ("Fixes", "Closes", "Related")))
    keyword_pattern = "|".join(re.escape(keyword) for keyword in keywords)
    valid_pattern = re.compile(
        rf"^(?:{keyword_pattern})[ \t]+#(?P<issue>[1-9][0-9]*)$",
        flags=re.IGNORECASE,
    )
    candidate_pattern = re.compile(
        rf"^(?:{keyword_pattern})[ \t]*#",
        flags=re.IGNORECASE,
    )
    links: list[dict[str, Any]] = []
    errors: list[str] = []
    for line in markdown_block_lines(body):
        stripped = line.strip()
        if not candidate_pattern.match(stripped):
            continue
        match = valid_pattern.fullmatch(stripped)
        if not match:
            errors.append("issue authority lines must use `Fixes|Closes|Related #<issue>`")
            continue
        links.append(
            {
                "issue": int(match.group("issue")),
            }
        )
    if AUTHORITY_LINKS.get("exactly_one", True) and len(links) > 1:
        errors.append("pull request must contain exactly one standalone issue authority line")
    return links, errors


def parse_related_issue(body: str) -> int | None:
    links, _ = parse_authority_links(body)
    if len(links) != 1:
        return None
    return int(links[0]["issue"])


def is_direct_low_risk(body: str) -> bool:
    pattern = re.compile(r"(?i)^\s*Direct low-risk PR:\s*\S")
    return any(pattern.search(line) for line in markdown_block_lines(body))


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



def parse_intent(
    body: str,
    issue: int | None = None,
    contract_root: Path | None = None,
) -> tuple[dict[str, Any] | None, str | None]:
    normalized = normalize_text(body)
    unfenced = "\n".join(markdown_block_lines(normalized))
    has_v1_marker = bool(
        re.search(r"(?m)^\*\*Intent accepted\*\*\s*$", unfenced)
        or re.search(r"(?m)^\s*repo-ops\.intent\.v1\b", unfenced)
    )
    if has_v1_marker:
        match = re.fullmatch(
            r"\*\*Intent accepted\*\*\n\n"
            r"(?P<outcome>.+?)\n\n"
            r"- Risk: `(?P<display_risk>low|medium|high)`\n"
            r"- Intent digest: `(?P<display_digest>sha256:[0-9a-f]{64})`\n"
            r"- Supersedes: `(?P<display_supersedes>none|sha256:[0-9a-f]{64})`\n\n"
            r"Authoritative machine-readable record:\n\n"
            r"```text\n(?:repo-ops\.intent\.v1[^\n]*)\n```\n",
            normalized,
            flags=re.DOTALL,
        )
        if not match:
            return None, "malformed repo-ops.intent.v1 accepted-intent comment"

        record, record_error = parse_record_line(body, "repo-ops.intent.v1")
        if record_error:
            return None, record_error
        if record is None:
            return None, "repo-ops.intent.v1 record is missing"
        schema_errors = validate_schema(
            record,
            (contract_root or CONTRACT_ROOT) / "intent.schema.json",
            "repo-ops.intent.v1",
        )
        if schema_errors:
            return None, schema_errors[0]
        if (
            issue is None
            or isinstance(issue, bool)
            or not isinstance(issue, int)
            or issue < 1
        ):
            return None, "repo-ops.intent.v1 requires a positive linked issue"

        outcome = match.group("outcome")
        computed = canonical_digest(
            {
                "issue": issue,
                "outcome": normalize_text(outcome),
                "risk": record["risk"],
            }
        )
        if match.group("display_risk") != record["risk"]:
            return None, "accepted-intent displayed risk does not match its raw record"
        if match.group("display_digest") != record["intent"]:
            return None, "accepted-intent displayed digest does not match its raw record"
        if match.group("display_supersedes") != record["supersedes"]:
            return None, "accepted-intent displayed supersession does not match its raw record"
        if record["intent"] != computed:
            return None, "accepted-intent digest does not match its canonical issue-bound payload"
        return {
            "kind": "v1",
            "digest": computed,
            "risk": record["risk"],
            "supersedes": record["supersedes"],
            "outcome": outcome,
        }, None

    has_legacy_marker = bool(re.search(r"(?m)^Intent accepted\.\s*$", unfenced))
    if not has_legacy_marker:
        return None, None
    match = re.fullmatch(
        r"Intent accepted\.\n\n(?P<outcome>.+?)\n\n"
        r"Risk: (?P<risk>low|medium|high)\n"
        r"Intent digest: (?P<digest>sha256:[0-9a-f]{64})\n",
        normalized,
        flags=re.DOTALL,
    )
    if not match:
        return None, "malformed legacy accepted-intent comment"
    computed = canonical_digest(
        {
            "outcome": normalize_text(match.group("outcome")),
            "risk": match.group("risk"),
        }
    )
    if computed != match.group("digest"):
        return None, "accepted-intent digest does not match its canonical payload"
    return {
        "kind": "legacy",
        "digest": computed,
        "risk": match.group("risk"),
        "outcome": match.group("outcome"),
    }, None




def parse_plan_approval(body: str) -> tuple[dict[str, Any] | None, str | None]:
    normalized = normalize_text(body)
    match = re.fullmatch(
        r"\*\*Plan approved(?: — [^\n]+)?\*\*\n\n"
        r"- Risk: `(?P<display_risk>low|medium|high)`\n"
        r"- Intent digest: `(?P<display_intent>sha256:[0-9a-f]{64})`\n"
        r"- Plan digest: `(?P<display_plan>sha256:[0-9a-f]{64})`\n"
        r"- Plan commit: `(?P<display_commit>[0-9a-f]{40})`\n\n"
        r"Authoritative machine-readable record:\n\n"
        r"```text\n"
        r"repo-ops\.plan-approval\.v1[^\n]*\n"
        r"```\n",
        normalized,
    )
    if not match:
        return None, "malformed repo-ops.plan-approval.v1 display wrapper"
    record, record_error = parse_record_line(body, "repo-ops.plan-approval.v1")
    if record_error:
        return None, record_error
    if record is None:
        return None, "repo-ops.plan-approval.v1 record is missing"
    displayed = {
        "risk": match.group("display_risk"),
        "intent": match.group("display_intent"),
        "plan": match.group("display_plan"),
        "plan-commit": match.group("display_commit"),
    }
    for field, value in displayed.items():
        if record.get(field) != value:
            return None, f"plan approval displayed {field} does not match its raw record"
    return record, None


def latest_record(
    entries: list[dict[str, Any]],
    record: str,
) -> tuple[dict[str, Any] | None, dict[str, Any] | None, list[str]]:
    found: list[tuple[dict[str, Any], dict[str, Any]]] = []
    errors: list[str] = []
    for entry in entries:
        body = entry.get("body") or ""
        if record not in body:
            continue
        if record == "repo-ops.plan-approval.v1":
            parsed, error = parse_plan_approval(body)
        else:
            parsed, error = parse_record_line(body, record)
        if error:
            errors.append(error)
        elif parsed:
            found.append((parsed, entry))
    if not found:
        return None, None, errors
    parsed, entry = found[-1]
    return parsed, entry, errors


def validate_state(
    state: dict[str, Any],
    contract_root: Path,
    *,
    check: str = "all",
) -> list[str]:
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
    authority_links, authority_errors = parse_authority_links(body)
    errors.extend(authority_errors)
    direct = is_direct_low_risk(body)
    if direct:
        if risk != "low":
            errors.append("only low-risk work may use the direct pull request route")
        if authority_links:
            errors.append("direct low-risk work must not include issue authority")
        if issue is not None:
            errors.append("direct low-risk work must not link an issue")
    elif not authority_errors:
        if len(authority_links) != 1:
            errors.append(
                "non-direct work must link an issue with `Fixes #<issue>` "
                "(or `Closes #<issue>` or `Related #<issue>`)"
            )
        elif issue is None:
            errors.append("issue authority requires a linked issue")
        elif authority_links[0]["issue"] != issue:
            errors.append("issue authority does not match the linked issue")
    declaration, declaration_errors = parse_changelog_declaration(body, contract_root)
    errors.extend(declaration_errors)
    errors.extend(
        validate_changelog_state(
            state,
            policy=policy,
            issue=issue,
            direct=direct,
            declaration=declaration,
        )
    )
    comments = state.get("comments") or []
    if issue is not None:
        issue_body = state.get("issue_body") or ""
        errors.extend(
            validate_heading_contract(issue_body, CONTRACT["issue"], "linked issue body")
        )
    reviews = state.get("reviews") or []
    authorized_comments = [entry for entry in comments if record_candidate(entry)]
    authorized_reviews = [entry for entry in reviews if record_candidate(entry)]
    records = [
        *({**entry, "surface": "comment"} for entry in authorized_comments),
        *({**entry, "surface": "review"} for entry in authorized_reviews),
    ]

    expected_intent = "none"
    if issue is not None:
        chain_head: dict[str, Any] | None = None
        saw_v1 = False
        for comment in state.get("intent_comments") or []:
            if not record_candidate(comment):
                continue
            parsed_intent, intent_error = parse_intent(
                comment.get("body") or "",
                issue,
                contract_root,
            )
            if intent_error:
                errors.append(intent_error)
                continue
            if parsed_intent is None:
                continue
            if not record_authorized(comment, record="repo-ops.intent.v1"):
                errors.append("accepted intent was not posted by an authorized maintainer")
                continue
            if parsed_intent["kind"] == "legacy":
                if saw_v1:
                    errors.append("legacy accepted intent cannot follow a v1 intent record")
                else:
                    chain_head = parsed_intent
                continue

            saw_v1 = True
            expected_supersedes = chain_head["digest"] if chain_head else "none"
            if parsed_intent["supersedes"] != expected_supersedes:
                errors.append("repo-ops.intent.v1 supersedes does not match the current intent")
                continue
            chain_head = parsed_intent

        if chain_head is None:
            errors.append("linked issue has no valid maintainer accepted-intent comment")
        else:
            expected_intent = chain_head["digest"]
            if chain_head["risk"] != risk:
                errors.append("pull request risk does not match accepted intent")

    expected_plan = state.get("plan_digest") or "none"
    if risk in {"medium", "high"} and expected_plan == "none":
        errors.append(f"{risk}-risk work requires a current plan")

    plan_comments = [entry for entry in comments if record_candidate(entry)]
    if risk in {"medium", "high"}:
        approval, approval_entry, approval_errors = latest_record(
            plan_comments,
            "repo-ops.plan-approval.v1",
        )
        errors.extend(approval_errors)
        if approval is None:
            errors.append(f"{risk}-risk work requires a maintainer plan approval")
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
            if approval_entry is None or not record_authorized(
                approval_entry,
                record="repo-ops.plan-approval.v1",
                risk=risk,
            ):
                errors.append(f"{risk}-risk plan approval was not posted by an authorized maintainer")
            if approval.get("plan-commit") not in (state.get("plan_commits") or []):
                errors.append("approved plan commit is not in the pull request history")

    if check != "contract":
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



            if receipt_entry and not record_authorized(
                receipt_entry,
                record="repo-ops.merge-review.v1",
            ):
                errors.append("merge review was not posted by an authorized maintainer")
            if policy.get("review", {}).get("shared_identity") is False:
                if receipt_entry and receipt_entry.get("surface") != "review":
                    errors.append("independent review receipt must be submitted as a native GitHub Review")
                if receipt_entry and receipt_entry.get("author") == pull_request.get("author"):
                    errors.append("independent review requires an identity distinct from the pull request author")

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


def collaborator_permission(
    client: GitHubClient,
    repository: str,
    login: str | None,
    cache: dict[str, str | None],
) -> str | None:
    if not login:
        return None
    if login in cache:
        return cache[login]
    try:
        result = client.get(
            f"/repos/{repository}/collaborators/{urllib.parse.quote(login, safe='')}/permission"
        )
    except RuntimeError:
        result = {}
    permission = result.get("permission") if isinstance(result, dict) else None
    cache[login] = permission if isinstance(permission, str) else None
    return cache[login]


def entry_from_api(item: dict[str, Any], *, permission: str | None = None) -> dict[str, Any]:
    return {
        "body": item.get("body") or "",
        "author": (item.get("user") or {}).get("login"),
        "association": item.get("author_association"),
        "permission": permission,
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
    file_items = client.get_all(
        f"/repos/{repository}/pulls/{number}/files?per_page=100"
    )
    files = [
        {
            "filename": item.get("filename"),
            "status": item.get("status"),
            "additions": item.get("additions"),
            "deletions": item.get("deletions"),
            "changes": item.get("changes"),
        }
        for item in file_items
        if isinstance(item.get("filename"), str)
    ]
    fragment_contents: dict[str, str] = {}
    fragment_root = (policy or {}).get("changelog", {}).get("root", "changelog.d")
    for entry in files:
        if (
            str(entry.get("status", "")).lower() in {"added", "modified"}
            and _fragment_path(entry["filename"], fragment_root)
        ):
            fragment_contents[entry["filename"]] = api_content(
                client,
                repository,
                entry["filename"],
                pull_request["head"]["sha"],
            )
    comparison = client.get(
        f"/repos/{repository}/compare/{base_sha}...{pull_request['head']['sha']}"
    )
    merge_base_sha = comparison["merge_base_commit"]["sha"]
    merge_base, diff_digest = git_metadata(pull_request, token, merge_base_sha)
    permission_cache: dict[str, str | None] = {}

    def hydrated_entry(
        item: dict[str, Any],
        *,
        permission_required: bool = False,
    ) -> dict[str, Any]:
        body = item.get("body") or ""
        login = (item.get("user") or {}).get("login")
        permission = None
        if permission_required or any(
            marker in body
            for marker in (
                "repo-ops.intent.v1",
                "repo-ops.plan-approval.v1",
                "repo-ops.merge-review.v1",
            )
        ):
            permission = collaborator_permission(client, repository, login, permission_cache)
        return entry_from_api(item, permission=permission)

    pr_comments = [
        hydrated_entry(item)
        for item in client.get_all(
            f"/repos/{repository}/issues/{number}/comments?per_page=100"
        )
    ]
    reviews = [
        hydrated_entry(item)
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
            hydrated_entry(item, permission_required=True)
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
        *pr_comments,
        *reviews,
    ):
        if not record_candidate(comment):
            continue
        approval, _ = parse_plan_approval(comment["body"])
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
            "files": files,
        },
        "files": files,
        "file_contents": fragment_contents,
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


def emit_findings(findings: list[str], check: str) -> None:
    if findings:
        for finding in findings:
            print(f"::warning title=Repository policy::{finding}")
        summary = os.environ.get("GITHUB_STEP_SUMMARY")
        if summary:
            with open(summary, "a", encoding="utf-8") as stream:
                stream.write(f"## Repository policy / {check} findings\n\n")
                for finding in findings:
                    stream.write(f"- {finding}\n")
        print(f"Repository policy / {check}: {len(findings)} finding(s)")
    else:
        print(f"Repository policy / {check}: contract satisfied")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--event", type=Path)
    parser.add_argument("--fixture", type=Path)
    parser.add_argument("--digest-out", type=Path)
    parser.add_argument(
        "--check",
        choices=("all", "contract", "merge-approval"),
        default="all",
    )

    args = parser.parse_args()
    if bool(args.fixture) == bool(args.event):
        parser.error("exactly one of --fixture or --event is required")
    repository_root = Path(__file__).resolve().parents[2]
    contract_root = repository_root / "contracts" / "v0.1.0"
    try:
        if args.fixture:
            state, _ = load_fixture(args.fixture)
        else:
            state = build_live_state(args.event)
        if args.digest_out:
            digest_evidence = {
                "plan_digest": state.get("plan_digest", "none"),
                "diff_digest": state.get("pull_request", {}).get("diff_digest"),
            }
            args.digest_out.write_text(
                json.dumps(digest_evidence, sort_keys=True) + "\n",
                encoding="utf-8",
            )
        if args.check == "merge-approval":
            contract_findings = validate_state(state, contract_root, check="contract")
            findings = validate_state(state, contract_root, check="merge-approval")
            if contract_findings:
                findings.insert(0, "contract check did not succeed; merge approval is blocked")
        else:
            findings = validate_state(state, contract_root, check=args.check)
    except (OSError, RuntimeError, ValueError, yaml.YAMLError) as error:
        findings = [str(error)]

    emit_findings(findings, args.check)
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main())
