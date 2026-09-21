#!/usr/bin/env python3
"""Authoring helper: write tests/testdata/events/validator bundles. Not a test."""

from __future__ import annotations

import base64
import hashlib
import json
import os
import shutil
import subprocess
import tempfile
from pathlib import Path
from typing import Any
from urllib.parse import quote

ROOT = Path(__file__).resolve().parents[3]
DEST = Path(__file__).resolve().parent / "validator"
POLICY_YAML = (ROOT / ".github" / "repo-policy.yml").read_text(encoding="utf-8")
REPO = "haesol-shin/.github"
CLONE = "https://github.com/haesol-shin/.github.git"
API = "https://api.github.com"
INTENT_DIGEST = "sha256:ee5673777e09f11f0b7c7f82039d80765828e84a1b0a94a7b68a4cb80df13871"
PLAN_TEXT = "Add a trusted validator without executing pull request code.\n"
PLAN_DIGEST = "sha256:05d8bc6233febe161f72fb8d87fe8614a339774e0698a4750309431dcbf9432c"
PLAN_PATH = ".ops/plans/42-replay.md"
FRAGMENT_PATH = "changelog.d/42-replay-evidence.md"
FRAGMENT_TEXT = "## Added\n- Add replay evidence for collector event bundles.\n"
ISSUE_BODY = (
    "## Problem\n\n"
    "Repository approvals are not machine-validated.\n\n"
    "## Desired outcome\n\n"
    "Trusted validation rejects stale or malformed evidence.\n\n"
    "## Work\n\n"
    "- Add trusted contract validation.\n\n"
    "## Acceptance\n\n"
    "- Stale and malformed evidence is rejected by conformance fixtures.\n\n"
    "## Non-goals\n\n"
    "- Executing untrusted pull request code.\n\n"
    "## Context\n\n"
    "High-risk repository policy change governed by issue #42.\n"
)
INTENT_BODY = (
    "Intent accepted.\n\n"
    "Add a trusted validator without executing pull request code.\n\n"
    "Risk: high\n"
    f"Intent digest: {INTENT_DIGEST}\n"
)
HEAD_POLICY = POLICY_YAML.replace("check: quality", "check: untrusted-quality")
BINARY = bytes([0x00, 0xFF, 0x10, 0x80, 0x00, 0x7F]) + b"blob"
QUOTED_PATH = "docs/quoted path/你好.bin"
DATE = "2024-01-01T00:00:00+0000"
DIFF_GIT = [
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
]


def write_text(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8", newline="\n")


def write_json(path: Path, payload: Any) -> None:
    write_text(path, json.dumps(payload, indent=2, ensure_ascii=False) + "\n")


def git_env() -> dict[str, str]:
    env = os.environ.copy()
    env.update(
        {
            "GIT_AUTHOR_NAME": "Replay",
            "GIT_AUTHOR_EMAIL": "replay@example.test",
            "GIT_COMMITTER_NAME": "Replay",
            "GIT_COMMITTER_EMAIL": "replay@example.test",
            "GIT_AUTHOR_DATE": DATE,
            "GIT_COMMITTER_DATE": DATE,
            "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_TERMINAL_PROMPT": "0",
        }
    )
    return env


def git(args: list[str], cwd: Path) -> str:
    completed = subprocess.run(
        args,
        cwd=cwd,
        env=git_env(),
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=True,
    )
    return completed.stdout.decode("utf-8").strip()


def sha1_blob(data: bytes) -> str:
    return hashlib.sha1(b"blob " + str(len(data)).encode("ascii") + b"\0" + data).hexdigest()


def github_content(path: str, text: str) -> dict[str, Any]:
    raw = text.encode("utf-8")
    encoded = base64.b64encode(raw).decode("ascii")
    wrapped = "\n".join(encoded[i : i + 60] for i in range(0, len(encoded), 60)) + "\n"
    return {
        "name": path.rsplit("/", 1)[-1],
        "path": path,
        "sha": sha1_blob(raw),
        "size": len(raw),
        "encoding": "base64",
        "content": wrapped,
        "type": "file",
    }


def pr_body() -> str:
    return (
        "## Summary\n\n"
        "Add the trusted repository policy validator.\n\n"
        "## Changes\n\n"
        "- Validate immutable approval and review receipts.\n\n"
        "## Impact\n\n"
        "- User/runtime: policy failures become visible before merge\n"
        "- API/schema/dependencies: adds the repository policy contract\n"
        "- Operations/deployment: no production deployment\n"
        "- Not changed: pull request heads remain untrusted input\n"
        "repo-ops.changelog.v1 kind:not-required value:receipt-validation\n\n"
        "## Verification\n\n"
        "- Conformance fixtures — local runner — passed\n"
        "- Unverified: none\n\n"
        "## Risk and rollback\n\n"
        "Risk: high — changes repository policy enforcement\n\n"
        "Rollback: revert the pull request\n\n"
        "## Related\n\n"
        "Fixes #42\n"
        f"Plan: {PLAN_PATH}\n"
    )


def edited_pr_body() -> str:
    return pr_body().replace(
        "Risk: high — changes repository policy enforcement",
        "Risk: medium — body authority edited after opening",
    )


def plan_approval(plan_commit: str) -> str:
    return (
        "**Plan approved**\n\n"
        "- Risk: `high`\n"
        f"- Intent digest: `{INTENT_DIGEST}`\n"
        f"- Plan digest: `{PLAN_DIGEST}`\n"
        f"- Plan commit: `{plan_commit}`\n\n"
        "Authoritative machine-readable record:\n\n"
        "```text\n"
        "repo-ops.plan-approval.v1 decision:approved risk:high issue:42 "
        f"intent:{INTENT_DIGEST} plan:{PLAN_DIGEST} plan-commit:{plan_commit}\n"
        "```\n"
    )


def stale_plan_approval(plan_commit: str) -> str:
    return plan_approval(plan_commit).replace(PLAN_DIGEST, "sha256:" + "e" * 64)


def merge_review(base: str, head: str, diff: str) -> str:
    return (
        "Ready.\n\n"
        "repo-ops.merge-review.v1 verdict:merge-ready risk:high "
        f"intent:{INTENT_DIGEST} plan:{PLAN_DIGEST} "
        f"base:{base} head:{head} diff:{diff} runtime:omp/0.1 model:provider/model"
    )


def user(login: str) -> dict[str, str]:
    return {"login": login}


def comment(cid: int, body: str, login: str = "maintainer") -> dict[str, Any]:
    return {
        "id": cid,
        "user": user(login),
        "author_association": "OWNER",
        "body": body,
    }


def review(rid: int, body: str, state: str) -> dict[str, Any]:
    return {
        "id": rid,
        "user": user("maintainer"),
        "author_association": "OWNER",
        "state": state,
        "body": body,
    }


def file_entry(filename: str, status: str, additions: int = 1, deletions: int = 0) -> dict[str, Any]:
    return {
        "filename": filename,
        "status": status,
        "additions": additions,
        "deletions": deletions,
        "changes": additions + deletions,
    }


def high_files() -> list[dict[str, Any]]:
    return [
        file_entry(FRAGMENT_PATH, "added", 2, 0),
        file_entry(PLAN_PATH, "added", 1, 0),
        file_entry("actions/repository-policy/validate.py", "modified", 2, 1),
        file_entry("actions/repository-policy/action.yml", "modified", 1, 1),
        file_entry("contracts/v0.1.0/contract.json", "modified", 1, 1),
        file_entry(".github/repo-policy.yml", "modified", 1, 1),
        file_entry("assets/blob.bin", "added", 1, 0),
        file_entry(QUOTED_PATH, "added", 1, 0),
        file_entry("obsolete.txt", "removed", 0, 1),
    ]


def rec(method: str, url: str, body: Any, headers: dict[str, str] | None = None, status: int = 200) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "method": method,
        "url": url,
        "status": status,
        "headers": {"Content-Type": "application/json", **(headers or {})},
        "body": body,
    }
    return payload


def contents_url(path: str, ref: str) -> str:
    return f"/repos/{REPO}/contents/{quote(path, safe='/')}?ref={quote(ref)}"


def permission_url(login: str) -> str:
    return f"/repos/{REPO}/collaborators/{quote(login, safe='')}/permission"


def compare_url(base: str, head: str) -> str:
    return f"/repos/{REPO}/compare/{base}...{head}"


def init_work(path: Path) -> None:
    if path.exists():
        shutil.rmtree(path)
    path.mkdir(parents=True)
    git(["git", "init", "--quiet", "-b", "main"], path)
    git(["git", "config", "core.autocrlf", "false"], path)
    git(["git", "config", "core.quotepath", "true"], path)
    hooks = path / ".git" / "hooks-disabled"
    hooks.mkdir(exist_ok=True)
    git(["git", "config", "core.hooksPath", str(hooks)], path)


def commit_all(path: Path, message: str) -> str:
    git(["git", "add", "-A"], path)
    git(["git", "commit", "--quiet", "-m", message], path)
    return git(["git", "rev-parse", "HEAD"], path)


def write_bytes(path: Path, data: bytes) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)


def write_base_tree(path: Path) -> None:
    write_text(path / ".github" / "repo-policy.yml", POLICY_YAML)
    write_text(path / "README.md", "trusted base\n")
    write_text(path / "obsolete.txt", "delete me\n")
    write_text(path / "actions" / "repository-policy" / "validate.py", "# trusted base validator\n")
    write_text(path / "actions" / "repository-policy" / "action.yml", "name: repository-policy\n")
    write_text(path / "contracts" / "v0.1.0" / "contract.json", '{"id":"repo-ops/v0.1.0"}\n')


def write_head_tree(path: Path) -> None:
    write_text(path / PLAN_PATH, PLAN_TEXT)
    write_text(path / FRAGMENT_PATH, FRAGMENT_TEXT)
    write_text(
        path / "actions" / "repository-policy" / "validate.py",
        "# untrusted head validator\nprint('HEAD_CODE_EXECUTED')\n",
    )
    write_text(
        path / "actions" / "repository-policy" / "action.yml",
        "name: untrusted-head-action\n",
    )
    write_text(path / "contracts" / "v0.1.0" / "contract.json", '{"id":"untrusted-head"}\n')
    write_text(path / ".github" / "repo-policy.yml", HEAD_POLICY)
    write_bytes(path / "assets" / "blob.bin", BINARY)
    write_bytes(path / QUOTED_PATH, BINARY)
    obsolete = path / "obsolete.txt"
    if obsolete.exists():
        obsolete.unlink()


def diff_digest(path: Path, base: str, head: str) -> str:
    process = subprocess.Popen(
        [*DIFF_GIT, base, head, "--"],
        cwd=path,
        env=git_env(),
        stdout=subprocess.PIPE,
        stderr=subprocess.DEVNULL,
    )
    assert process.stdout is not None
    digest = hashlib.sha256()
    while chunk := process.stdout.read(1024 * 1024):
        digest.update(chunk)
    if process.wait():
        raise RuntimeError("git diff failed")
    return f"sha256:{digest.hexdigest()}"


def export_bundle(work: Path, dest: Path, pull_number: int, head: str) -> None:
    git(["git", "update-ref", f"refs/pull/{pull_number}/head", head], work)
    bundle = dest / "git" / "repo.bundle"
    bundle.parent.mkdir(parents=True, exist_ok=True)
    git(
        [
            "git",
            "bundle",
            "create",
            str(bundle),
            "HEAD",
            "main",
            f"refs/pull/{pull_number}/head",
        ],
        work,
    )


def pull_object(
    number: int,
    body: str,
    base: str,
    head: str,
    login: str = "contributor",
) -> dict[str, Any]:
    repo = {
        "full_name": REPO,
        "clone_url": CLONE,
    }
    return {
        "number": number,
        "title": "Add trusted validator",
        "body": body,
        "user": user(login),
        "head": {"sha": head, "ref": "replay-head", "repo": repo},
        "base": {"sha": base, "ref": "main", "repo": repo},
    }


def webhook_pr(event_name: str, action: str, pull: dict[str, Any], extra: dict[str, Any] | None = None) -> dict[str, Any]:
    payload = {
        "action": action,
        "number": pull["number"],
        "pull_request": pull,
        "repository": {
            "full_name": REPO,
            "clone_url": CLONE,
        },
        "sender": user("contributor"),
        "installation": {"id": "REDACTED"},
    }
    if extra:
        payload.update(extra)
    del event_name
    return payload


def webhook_comment(
    action: str,
    pull: dict[str, Any],
    comment_obj: dict[str, Any],
    changes: dict[str, Any] | None = None,
) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "action": action,
        "comment": comment_obj,
        "issue": {
            "number": pull["number"],
            "title": pull["title"],
            "body": pull["body"],
            "pull_request": {
                "url": f"{API}/repos/{REPO}/pulls/{pull['number']}",
            },
        },
        "repository": {"full_name": REPO, "clone_url": CLONE},
        "sender": user("maintainer"),
        "installation": {"id": "REDACTED"},
    }
    if changes is not None:
        payload["changes"] = changes
    return payload


def common_http(
    *,
    number: int,
    base: str,
    head: str,
    plan_commit: str,
    files: list[dict[str, Any]],
    comments: list[dict[str, Any]],
    reviews: list[dict[str, Any]],
    include_issue: bool,
    include_plan: bool,
    paginate_files: bool,
    paginate_checks: bool,
    quality_name: str = "quality",
) -> list[dict[str, Any]]:
    recordings = [
        rec("GET", contents_url(".github/repo-policy.yml", base), github_content(".github/repo-policy.yml", POLICY_YAML)),
    ]
    files_url = f"/repos/{REPO}/pulls/{number}/files?per_page=100"
    if paginate_files and len(files) >= 2:
        recordings.append(
            rec(
                "GET",
                files_url,
                [files[0]],
                {"Link": f'<{API}/repos/{REPO}/pulls/{number}/files?page=2&per_page=100>; rel="next"'},
            )
        )
        recordings.append(
            rec(
                "GET",
                f"/repos/{REPO}/pulls/{number}/files?page=2&per_page=100",
                files[1:],
            )
        )
    else:
        recordings.append(rec("GET", files_url, files))
    for entry in files:
        if str(entry.get("status", "")).lower() in {"added", "modified"} and str(entry["filename"]).startswith("changelog.d/"):
            recordings.append(
                rec(
                    "GET",
                    contents_url(entry["filename"], head),
                    github_content(entry["filename"], FRAGMENT_TEXT),
                )
            )
    recordings.append(
        rec(
            "GET",
            compare_url(base, head),
            {
                "status": "ahead" if base != head else "identical",
                "ahead_by": 0 if base == head else 1,
                "behind_by": 0,
                "merge_base_commit": {"sha": base},
            },
        )
    )
    recordings.append(
        rec("GET", f"/repos/{REPO}/issues/{number}/comments?per_page=100", comments)
    )
    recordings.append(
        rec("GET", f"/repos/{REPO}/pulls/{number}/reviews?per_page=100", reviews)
    )
    if include_issue:
        recordings.append(
            rec(
                "GET",
                f"/repos/{REPO}/issues/42",
                {"number": 42, "title": "Trusted validator", "body": ISSUE_BODY},
            )
        )
        recordings.append(
            rec(
                "GET",
                f"/repos/{REPO}/issues/42/comments?per_page=100",
                [comment(900, INTENT_BODY)],
            )
        )
        recordings.append(
            rec(
                "GET",
                permission_url("maintainer"),
                {"permission": "admin", "user": user("maintainer")},
            )
        )
    if include_plan:
        recordings.append(rec("GET", contents_url(PLAN_PATH, head), github_content(PLAN_PATH, PLAN_TEXT)))
        recordings.append(
            rec(
                "GET",
                compare_url(plan_commit, head),
                {
                    "status": "ahead" if plan_commit != head else "identical",
                    "ahead_by": 0 if plan_commit == head else 1,
                    "behind_by": 0,
                    "merge_base_commit": {"sha": plan_commit},
                },
            )
        )
        recordings.append(
            rec("GET", contents_url(PLAN_PATH, plan_commit), github_content(PLAN_PATH, PLAN_TEXT))
        )
    quality = {"name": quality_name, "conclusion": "success", "head_sha": head}
    checks_url = f"/repos/{REPO}/commits/{head}/check-runs?per_page=100"
    if paginate_checks:
        recordings.append(
            rec(
                "GET",
                checks_url,
                {
                    "total_count": 2,
                    "check_runs": [{"name": "other", "conclusion": "success", "head_sha": head}],
                },
                {"Link": f'<{API}/repos/{REPO}/commits/{head}/check-runs?page=2&per_page=100>; rel="next"'},
            )
        )
        recordings.append(
            rec(
                "GET",
                f"/repos/{REPO}/commits/{head}/check-runs?page=2&per_page=100",
                {"total_count": 2, "check_runs": [quality]},
            )
        )
    else:
        recordings.append(rec("GET", checks_url, {"total_count": 1, "check_runs": [quality]}))
    if not include_issue:
        recordings.append(
            rec(
                "GET",
                permission_url("maintainer"),
                {"permission": "admin", "user": user("maintainer")},
            )
        )
    return recordings


def write_bundle(
    name: str,
    *,
    work: Path,
    pull_number: int,
    base: str,
    head: str,
    plan_commit: str,
    diff: str,
    webhook: dict[str, Any],
    recordings: list[dict[str, Any]],
    extra_recordings: list[dict[str, Any]] | None = None,
) -> None:
    dest = DEST / name
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    export_bundle(work, dest, pull_number, head)
    write_json(
        dest / "git" / "meta.json",
        {
            "pull_number": pull_number,
            "base": base,
            "head": head,
            "plan_commit": plan_commit,
            "plan_digest": PLAN_DIGEST if PLAN_PATH else "none",
            "diff_digest": diff,
        },
    )
    write_json(dest / "webhook.json", webhook)
    all_recs = list(recordings)
    if extra_recordings:
        all_recs.extend(extra_recordings)
    write_json(dest / "http" / "recordings.json", all_recs)


def build_high(tmp: Path) -> dict[str, str]:
    work = tmp / "high"
    init_work(work)
    write_base_tree(work)
    base = commit_all(work, "base")
    write_text(work / PLAN_PATH, PLAN_TEXT)
    plan_commit = commit_all(work, "plan")
    write_head_tree(work)
    head = commit_all(work, "head")
    write_text(work / "README.md", "trusted base\nsynchronize\n")
    sync = commit_all(work, "synchronize")
    git(["git", "update-ref", "refs/heads/head", head], work)
    git(["git", "update-ref", "refs/heads/sync", sync], work)
    diff_head = diff_digest(work, base, head)
    diff_sync = diff_digest(work, base, sync)
    return {
        "work": str(work),
        "base": base,
        "plan_commit": plan_commit,
        "head": head,
        "sync": sync,
        "diff_head": diff_head,
        "diff_sync": diff_sync,
    }


def build_empty(tmp: Path) -> dict[str, str]:
    work = tmp / "empty"
    init_work(work)
    write_base_tree(work)
    base = commit_all(work, "base")
    git(["git", "commit", "--quiet", "--allow-empty", "-m", "empty"], work)
    head = git(["git", "rev-parse", "HEAD"], work)
    diff = diff_digest(work, base, head)
    return {"work": str(work), "base": base, "head": head, "diff": diff, "plan_commit": base}


def checkout(work: Path, sha: str) -> None:
    git(["git", "checkout", "--quiet", sha], work)


def main() -> None:
    DEST.mkdir(parents=True, exist_ok=True)
    tmp = Path(tempfile.mkdtemp(prefix="repo-ops-bundles-"))
    try:
        high = build_high(tmp)
        empty = build_empty(tmp)
        high_work = Path(high["work"])
        empty_work = Path(empty["work"])
        base = high["base"]
        plan_commit = high["plan_commit"]
        head = high["head"]
        sync = high["sync"]
        diff_head = high["diff_head"]
        diff_sync = high["diff_sync"]
        body = pr_body()
        edited_body = edited_pr_body()
        approval = plan_approval(plan_commit)
        receipt = merge_review(base, head, diff_head)
        stale_receipt = merge_review(base, head, diff_head).replace(head, "a" * 40)
        stale_approval = stale_plan_approval(plan_commit)

        def high_pull(number: int, pr_body_text: str, head_sha: str) -> dict[str, Any]:
            return pull_object(number, pr_body_text, base, head_sha)

        # opened: valid high-risk, pagination, trust mutations, git evidence
        opened_pull = high_pull(201, body, head)
        checkout(high_work, head)
        write_bundle(
            "pull_request_target-opened",
            work=high_work,
            pull_number=201,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_pr("pull_request_target", "opened", opened_pull),
            recordings=common_http(
                number=201,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[comment(1, approval), comment(2, receipt)],
                reviews=[],
                include_issue=True,
                include_plan=True,
                paginate_files=True,
                paginate_checks=True,
            ),
        )

        # edited: risk medium vs high intent
        edited_pull = high_pull(202, edited_body, head)
        edited_pull["title"] = "Edited title after opening"
        write_bundle(
            "pull_request_target-edited",
            work=high_work,
            pull_number=202,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_pr(
                "pull_request_target",
                "edited",
                edited_pull,
                extra={"changes": {"title": {"from": "Add trusted validator"}, "body": {"from": body}}},
            ),
            recordings=common_http(
                number=202,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[comment(1, approval), comment(2, receipt)],
                reviews=[],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            ),
        )

        # reopened: same as opened without pagination
        reopened_pull = high_pull(203, body, head)
        write_bundle(
            "pull_request_target-reopened",
            work=high_work,
            pull_number=203,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_pr("pull_request_target", "reopened", reopened_pull),
            recordings=common_http(
                number=203,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[comment(1, approval), comment(2, receipt)],
                reviews=[],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            ),
        )

        # synchronize: new head, stale merge-review head
        checkout(high_work, sync)
        sync_files = high_files() + [file_entry("README.md", "modified", 1, 0)]
        sync_pull = high_pull(204, body, sync)
        write_bundle(
            "pull_request_target-synchronize",
            work=high_work,
            pull_number=204,
            base=base,
            head=sync,
            plan_commit=plan_commit,
            diff=diff_sync,
            webhook=webhook_pr(
                "pull_request_target",
                "synchronize",
                sync_pull,
                extra={"before": head, "after": sync},
            ),
            recordings=common_http(
                number=204,
                base=base,
                head=sync,
                plan_commit=plan_commit,
                files=sync_files,
                comments=[comment(1, approval), comment(2, receipt)],
                reviews=[],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            ),
        )
        checkout(high_work, head)

        # review submitted: receipt on reviews
        review_pull = high_pull(205, body, head)
        write_bundle(
            "pull_request_review-submitted",
            work=high_work,
            pull_number=205,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_pr(
                "pull_request_review",
                "submitted",
                review_pull,
                extra={"review": review(11, receipt, "APPROVED")},
            ),
            recordings=common_http(
                number=205,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[comment(1, approval)],
                reviews=[review(11, receipt, "APPROVED")],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            ),
        )

        # review edited: stale review body
        edited_review_pull = high_pull(206, body, head)
        write_bundle(
            "pull_request_review-edited",
            work=high_work,
            pull_number=206,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_pr(
                "pull_request_review",
                "edited",
                edited_review_pull,
                extra={
                    "review": review(12, stale_receipt, "APPROVED"),
                    "changes": {"body": {"from": receipt}},
                },
            ),
            recordings=common_http(
                number=206,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[comment(1, approval)],
                reviews=[review(12, stale_receipt, "APPROVED")],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            ),
        )

        # issue comment created: merge-review comment
        created_pull = high_pull(208, body, head)
        created_comment = comment(21, receipt)
        write_bundle(
            "issue_comment-created-pr",
            work=high_work,
            pull_number=208,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_comment("created", created_pull, created_comment),
            recordings=common_http(
                number=208,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[comment(1, approval), created_comment],
                reviews=[],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            )
            + [rec("GET", f"/repos/{REPO}/pulls/208", created_pull)],
        )

        # issue comment edited: stale approval
        edited_c_pull = high_pull(209, body, head)
        edited_comment = comment(22, stale_approval)
        write_bundle(
            "issue_comment-edited-pr",
            work=high_work,
            pull_number=209,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_comment(
                "edited",
                edited_c_pull,
                edited_comment,
                changes={"body": {"from": approval}},
            ),
            recordings=common_http(
                number=209,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[edited_comment, comment(2, receipt)],
                reviews=[],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            )
            + [rec("GET", f"/repos/{REPO}/pulls/209", edited_c_pull)],
        )

        # issue comment deleted: approval gone
        deleted_pull = high_pull(210, body, head)
        deleted_comment = comment(23, approval)
        write_bundle(
            "issue_comment-deleted-pr",
            work=high_work,
            pull_number=210,
            base=base,
            head=head,
            plan_commit=plan_commit,
            diff=diff_head,
            webhook=webhook_comment("deleted", deleted_pull, deleted_comment),
            recordings=common_http(
                number=210,
                base=base,
                head=head,
                plan_commit=plan_commit,
                files=high_files(),
                comments=[comment(2, receipt)],
                reviews=[],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            )
            + [rec("GET", f"/repos/{REPO}/pulls/210", deleted_pull)],
        )

        # dismissed + empty diff
        empty_base = empty["base"]
        empty_head = empty["head"]
        empty_diff = empty["diff"]
        empty_body = body
        dismissed_pull = pull_object(207, empty_body, empty_base, empty_head)
        dismissed_receipt = merge_review(empty_base, empty_head, empty_diff)
        dismissed_approval = plan_approval(empty["plan_commit"])
        write_bundle(
            "pull_request_review-dismissed",
            work=empty_work,
            pull_number=207,
            base=empty_base,
            head=empty_head,
            plan_commit=empty["plan_commit"],
            diff=empty_diff,
            webhook=webhook_pr(
                "pull_request_review",
                "dismissed",
                dismissed_pull,
                extra={"review": review(13, dismissed_receipt, "DISMISSED")},
            ),
            recordings=common_http(
                number=207,
                base=empty_base,
                head=empty_head,
                plan_commit=empty["plan_commit"],
                files=[],
                comments=[comment(1, dismissed_approval)],
                reviews=[review(13, dismissed_receipt, "DISMISSED")],
                include_issue=True,
                include_plan=True,
                paginate_files=False,
                paginate_checks=False,
            ),
        )

        print("wrote", DEST)
        for name, info in (("high", high), ("empty", empty)):
            print(name, json.dumps({k: v for k, v in info.items() if k != "work"}, indent=2))
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    main()
