#!/usr/bin/env python3
"""Stage 2 validator replay: local HTTP + local git, Python/Go CLI matrix.

Does not call live GitHub or the caller's working tree as a git remote.
Does not load tests/testdata/events/workflow bundles.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlparse, urlunparse

ROOT = Path(__file__).resolve().parents[3]
EVENTS = Path(__file__).resolve().parent
VALIDATOR_ROOT = EVENTS / "validator"
VALIDATOR_PY = ROOT / "actions" / "repository-policy" / "validate.py"
ANNOTATION_PREFIX = "::warning title=Repository policy::"
CHECKS = ("all", "contract", "merge-approval")
SECRET_VALUE = "repo-ops-stage2-replay-token-c4e8a1b9d7f30526"
API_HOST_MARKERS = (
    "https://api.github.com",
    "http://api.github.com",
)
CLONE_MARKERS = (
    "https://github.com/haesol-shin/.github.git",
    "https://github.com/haesol-shin/.github",
    "git://github.com/haesol-shin/.github.git",
)
HEAD_EXEC_MARKERS = ("HEAD_CODE_EXECUTED", "untrusted-quality")
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


def python_cmd() -> list[str]:
    uv = shutil.which("uv")
    if uv:
        return [
            uv,
            "run",
            "--project",
            str(ROOT / "actions" / "repository-policy"),
            "--locked",
            "python",
            str(VALIDATOR_PY),
        ]
    return [sys.executable, str(VALIDATOR_PY)]


def go_cmd() -> list[str]:
    override = os.environ.get("REPO_OPS_VALIDATOR")
    if override:
        return [override]
    return ["go", "run", "./cmd/repo-ops-validator"]


def parse_findings(stdout: str) -> list[str]:
    findings: list[str] = []
    for line in stdout.splitlines():
        if line.startswith(ANNOTATION_PREFIX):
            findings.append(line[len(ANNOTATION_PREFIX) :])
    return findings


def semantic(exit_code: int, findings: list[str]) -> dict[str, Any]:
    return {
        "exit": exit_code,
        "findings": findings,
        "check_conclusion": "success" if exit_code == 0 else "failure",
    }


def bundle_dirs() -> list[Path]:
    if not VALIDATOR_ROOT.is_dir():
        return []
    return sorted(
        path
        for path in VALIDATOR_ROOT.iterdir()
        if path.is_dir() and (path / "webhook.json").is_file()
    )


def normalize_url(url: str) -> str:
    parsed = urlparse(url)
    path = parsed.path or "/"
    query = urlencode(sorted(parse_qsl(parsed.query, keep_blank_values=True)))
    return urlunparse(("", "", path, "", query, ""))


def load_recordings(bundle: Path) -> list[dict[str, Any]]:
    path = bundle / "http" / "recordings.json"
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, list):
        raise SystemExit(f"{path} must be a JSON array")
    return payload


def recording_key(method: str, url: str) -> tuple[str, str]:
    return method.upper(), normalize_url(url)


def rewrite_link(value: str, origin: str) -> str:
    rewritten = value.replace("{api}", origin.rstrip("/"))
    for marker in API_HOST_MARKERS:
        rewritten = rewritten.replace(marker, origin.rstrip("/"))
    return rewritten


class ReplayHandler(BaseHTTPRequestHandler):
    recordings: dict[tuple[str, str], dict[str, Any]]
    origin: str
    clone_url: str
    seen: list[str]
    unmatched: list[str]

    def log_message(self, format: str, *args: Any) -> None:
        del format, args

    def do_GET(self) -> None:  # noqa: N802
        self._serve("GET")

    def do_POST(self) -> None:  # noqa: N802
        self._serve("POST")

    def _serve(self, method: str) -> None:
        parsed = urlparse(self.path)
        query = urlencode(sorted(parse_qsl(parsed.query, keep_blank_values=True)))
        url = urlunparse(("", "", parsed.path, "", query, ""))
        key = recording_key(method, url)
        self.seen.append(f"{method} {url}")
        rec = self.recordings.get(key)
        if rec is None:
            self.unmatched.append(f"{method} {self.path}")
            body = json.dumps({"message": "not recorded", "url": self.path}).encode("utf-8")
            self.send_response(404)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        raw_body = rec.get("body", {})
        if isinstance(raw_body, (dict, list)):
            rewritten = rewrite_event(raw_body, origin=self.origin, clone_url=self.clone_url)
            payload = json.dumps(rewritten, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
        elif isinstance(raw_body, str):
            payload = str(rewrite_event(raw_body, origin=self.origin, clone_url=self.clone_url)).encode("utf-8")
        else:
            payload = b""
        status = int(rec.get("status", 200))
        headers = dict(rec.get("headers") or {})
        self.send_response(status)
        self.send_header("Content-Type", headers.pop("Content-Type", "application/json"))
        self.send_header("Content-Length", str(len(payload)))
        for name, value in headers.items():
            if name.lower() == "link":
                value = rewrite_link(str(value), self.origin)
            self.send_header(name, str(value))
        self.end_headers()
        self.wfile.write(payload)


def start_http(
    recordings: list[dict[str, Any]],
    *,
    clone_url: str,
) -> tuple[ThreadingHTTPServer, dict[tuple[str, str], dict[str, Any]], list[str], list[str]]:
    index: dict[tuple[str, str], dict[str, Any]] = {}
    for rec in recordings:
        index[recording_key(str(rec["method"]), str(rec["url"]))] = rec
    seen: list[str] = []
    unmatched: list[str] = []

    class Bound(ReplayHandler):
        pass

    Bound.recordings = index
    Bound.seen = seen
    Bound.unmatched = unmatched
    Bound.clone_url = clone_url
    server = ThreadingHTTPServer(("127.0.0.1", 0), Bound)
    origin = f"http://127.0.0.1:{server.server_address[1]}"
    Bound.origin = origin
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server, index, seen, unmatched


def git_run(args: list[str], *, cwd: Path, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[bytes]:
    merged = os.environ.copy()
    if env:
        merged.update(env)
    merged.setdefault("GIT_CONFIG_NOSYSTEM", "1")
    merged.setdefault("GIT_TERMINAL_PROMPT", "0")
    return subprocess.run(args, cwd=cwd, env=merged, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True)


def hydrate_git(bundle: Path, dest: Path, pull_number: int) -> None:
    git_dir = bundle / "git"
    bundle_file = git_dir / "repo.bundle"
    if not bundle_file.is_file():
        raise SystemExit(f"missing git bundle: {bundle_file}")
    dest.mkdir(parents=True, exist_ok=True)
    git_run(["git", "init", "--bare", "--quiet", str(dest)], cwd=ROOT)
    git_run(
        [
            "git",
            "--git-dir",
            str(dest),
            "fetch",
            "--quiet",
            str(bundle_file),
            f"refs/pull/{pull_number}/head:refs/pull/{pull_number}/head",
        ],
        cwd=ROOT,
    )

def independent_diff_digest(remote: Path, base: str, head: str, pull_number: int) -> str:
    work = Path(tempfile.mkdtemp(prefix="repo-ops-replay-hash-"))
    try:
        git_run(["git", "init", "--quiet", str(work)], cwd=ROOT)
        git_run(
            [
                "git",
                "fetch",
                "--quiet",
                "--no-tags",
                str(remote),
                f"refs/pull/{pull_number}/head:refs/repo-ops/head",
            ],
            cwd=work,
        )
        fetched = git_run(["git", "rev-parse", "refs/repo-ops/head"], cwd=work).stdout.decode().strip()
        if fetched != head:
            raise SystemExit(f"hydrated head {fetched} != {head}")
        git_run(["git", "cat-file", "-e", f"{base}^{{commit}}"], cwd=work)
        git_run(["git", "update-ref", "refs/repo-ops/base", base], cwd=work)
        process = subprocess.Popen(
            [*DIFF_GIT, "refs/repo-ops/base", "refs/repo-ops/head", "--"],
            cwd=work,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
        )
        assert process.stdout is not None
        digest = hashlib.sha256()
        while chunk := process.stdout.read(1024 * 1024):
            digest.update(chunk)
        if process.wait():
            raise SystemExit("independent git diff failed")
        return f"sha256:{digest.hexdigest()}"
    finally:
        shutil.rmtree(work, ignore_errors=True)


def rewrite_event(event: Any, *, origin: str, clone_url: str) -> Any:
    if isinstance(event, dict):
        return {key: rewrite_event(value, origin=origin, clone_url=clone_url) for key, value in event.items()}
    if isinstance(event, list):
        return [rewrite_event(item, origin=origin, clone_url=clone_url) for item in event]
    if isinstance(event, str):
        if event in CLONE_MARKERS or event.endswith(".git") and "github.com" in event:
            return clone_url
        for marker in API_HOST_MARKERS:
            if event.startswith(marker):
                return origin.rstrip("/") + event[len(marker) :]
        if "REDACTED" in event:
            return event
    return event

def load_git_meta(bundle: Path) -> dict[str, Any]:
    path = bundle / "git" / "meta.json"
    return json.loads(path.read_text(encoding="utf-8"))


def child_env(origin: str, summary: Path | None = None) -> dict[str, str]:
    env = os.environ.copy()
    env["GITHUB_TOKEN"] = SECRET_VALUE
    env["GITHUB_API_URL"] = origin
    env["GITHUB_REPOSITORY"] = "haesol-shin/.github"
    env["CGO_ENABLED"] = "0"
    env["GIT_TERMINAL_PROMPT"] = "0"
    env["GIT_CONFIG_NOSYSTEM"] = "1"
    if summary is not None:
        env["GITHUB_STEP_SUMMARY"] = str(summary)
    else:
        env.pop("GITHUB_STEP_SUMMARY", None)
    return env


def scan_secret(*texts: str, extra: Path | None = None) -> None:
    blobs = list(texts)
    if extra is not None and extra.is_file():
        blobs.append(extra.read_text(encoding="utf-8", errors="replace"))
    for blob in blobs:
        if SECRET_VALUE in blob:
            raise SystemExit("injected secret value leaked")
        for marker in HEAD_EXEC_MARKERS:
            if marker in blob:
                raise SystemExit(f"untrusted head marker leaked or executed: {marker}")


def capture(
    cmd: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    dest: Path,
    step_summary: bool = False,
) -> dict[str, Any]:
    dest.mkdir(parents=True, exist_ok=True)
    child = env.copy()
    summary = dest / "summary.md"
    if step_summary:
        child["GITHUB_STEP_SUMMARY"] = str(summary)
    else:
        child.pop("GITHUB_STEP_SUMMARY", None)
    completed = subprocess.run(
        cmd,
        cwd=cwd,
        env=child,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    stdout = completed.stdout.decode("utf-8", errors="replace")
    stderr = completed.stderr.decode("utf-8", errors="replace")
    write_text(dest / "stdout.txt", stdout)
    write_text(dest / "stderr.txt", stderr)
    write_text(dest / "exit.txt", f"{completed.returncode}\n")
    payload = semantic(completed.returncode, parse_findings(stdout))
    write_json(dest / "semantic.json", payload)
    scan_secret(stdout, stderr, extra=summary if step_summary else None)
    return payload


def expected_shape(row: dict[str, Any], plan_digest: str, diff_digest: str) -> dict[str, Any]:
    return {
        "exit": row["exit"],
        "findings": row["findings"],
        "check_conclusion": row["check_conclusion"],
        "plan_digest": plan_digest,
        "diff_digest": diff_digest,
    }


def compare_rows(label: str, observed: dict[str, Any], expected: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    for key in ("exit", "findings", "check_conclusion", "plan_digest", "diff_digest"):
        if observed.get(key) != expected.get(key):
            errors.append(f"{label} {key} mismatch")
    return errors


def run_bundle(
    bundle: Path,
    *,
    impls: tuple[str, ...],
    out: Path,
    write_expected: bool,
) -> list[str]:
    errors: list[str] = []
    event = json.loads((bundle / "webhook.json").read_text(encoding="utf-8"))
    recordings = load_recordings(bundle)
    meta = load_git_meta(bundle)
    pull_number = int(meta["pull_number"])
    base_sha = str(meta["base"])
    head_sha = str(meta["head"])
    plan_digest = str(meta["plan_digest"])
    work = Path(tempfile.mkdtemp(prefix="repo-ops-replay-"))
    server = None
    try:
        remote = work / "remote.git"
        hydrate_git(bundle, remote, pull_number)
        hashed = independent_diff_digest(remote, base_sha, head_sha, pull_number)
        expected_diff = str(meta["diff_digest"])
        if hashed != expected_diff:
            errors.append(f"{bundle.name} independent diff_digest {hashed} != {expected_diff}")
        clone_url = str(remote)
        server, _index, seen, unmatched = start_http(recordings, clone_url=clone_url)
        origin = f"http://127.0.0.1:{server.server_address[1]}"
        live_event = rewrite_event(event, origin=origin, clone_url=clone_url)
        event_path = work / "event.json"
        write_json(event_path, live_event)
        env = child_env(origin)
        observed: dict[str, dict[str, dict[str, Any]]] = {}
        for impl in impls:
            cmd = python_cmd() if impl == "python" else go_cmd()
            observed[impl] = {}
            for check in CHECKS:
                rel = f"{bundle.name}/{check}"
                row = capture(
                    [*cmd, "--event", str(event_path), "--check", check],
                    cwd=ROOT,
                    env=env,
                    dest=out / impl / rel,
                )
                capture(
                    [*cmd, "--event", str(event_path), "--check", check],
                    cwd=ROOT,
                    env=env,
                    dest=out / "summary-scan" / impl / rel,
                    step_summary=True,
                )
                observed[impl][check] = expected_shape(row, plan_digest, hashed)
        if unmatched:
            errors.append(f"{bundle.name} unmatched HTTP: {unmatched}")
        del seen
        expected_path = bundle / "expected.json"
        if write_expected:
            write_json(expected_path, observed["python"])
            committed = observed["python"]
        else:
            committed = json.loads(expected_path.read_text(encoding="utf-8"))
        for check in CHECKS:
            gold = committed[check]
            for impl in impls:
                errors.extend(compare_rows(f"{bundle.name}/{impl}/{check}", observed[impl][check], gold))
            if len(impls) > 1:
                first, second = impls[0], impls[1]
                if observed[first][check] != observed[second][check]:
                    errors.append(f"{bundle.name}/{check} python/go divergence")
    finally:
        if server is not None:
            server.shutdown()
            server.server_close()
        shutil.rmtree(work, ignore_errors=True)
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description="Stage 2 validator event replay")
    parser.add_argument("--out", type=Path, default=EVENTS / "observed")
    parser.add_argument("--write-expected", action="store_true")
    parser.add_argument("--impls", default="python,go")
    parser.add_argument("--bundle", action="append", default=[])
    args = parser.parse_args()
    impls = tuple(name.strip() for name in args.impls.split(",") if name.strip())
    args.out.mkdir(parents=True, exist_ok=True)
    selected = bundle_dirs()
    if args.bundle:
        wanted = set(args.bundle)
        selected = [path for path in selected if path.name in wanted]
    if not selected:
        raise SystemExit("no validator bundles found")
    errors: list[str] = []
    for bundle in selected:
        errors.extend(
            run_bundle(
                bundle,
                impls=impls,
                out=args.out,
                write_expected=args.write_expected,
            )
        )
    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        return 1
    print(f"replay ok: {len(selected)} bundle(s) × {len(CHECKS)} checks × {len(impls)} impl(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
