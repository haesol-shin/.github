#!/usr/bin/env python3
"""Collect local validator cold-start wall time and peak RSS.

Measures a prebuilt Go binary and a warmed `uv run` Python process against
`--fixture fixtures/valid-high.json --check all`. It neither contacts GitHub
nor measures clean-CI. Output is local-only and cannot overwrite the canonical
Stage 2 evidence record.
"""

from __future__ import annotations

import argparse
import json
import math
import os
import platform
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Callable

N_DEFAULT = 5
FIXTURE = Path("fixtures") / "valid-high.json"
RECORD_NAME = "5-go-validator.json"


def nearest_rank(samples: list[int], percentile: float) -> int:
    ordered = sorted(samples)
    n = len(ordered)
    if n == 0:
        return 0
    rank = max(1, min(n, math.ceil(percentile * n)))
    return ordered[rank - 1]


def summarize(samples: list[int]) -> dict[str, object]:
    return {
        "samples": list(samples),
        "p50": nearest_rank(samples, 0.50),
        "p95": nearest_rank(samples, 0.95),
        "n": len(samples),
    }


def repo_root_from(start: Path) -> Path:
    for candidate in (start, *start.parents):
        if (candidate / "go.mod").is_file() and (candidate / "fixtures").is_dir():
            return candidate
    raise SystemExit(f"could not find repository root from {start}")


def run_checked(command: list[str], *, cwd: Path, env: dict[str, str]) -> None:
    completed = subprocess.run(command, cwd=cwd, env=env, check=False)
    if completed.returncode != 0:
        raise SystemExit(f"command failed ({completed.returncode}): {command}")


class _WindowsRssTracker:
    _PROCESS_QUERY_INFORMATION = 0x0400
    _PROCESS_QUERY_LIMITED = 0x1000
    _PROCESS_VM_READ = 0x0010
    _TH32CS_SNAPPROCESS = 0x00000002

    def __init__(self, root_pid: int) -> None:
        import ctypes
        from ctypes import wintypes

        self._ctypes = ctypes
        self._wintypes = wintypes
        self._kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
        self._psapi = ctypes.WinDLL("psapi", use_last_error=True)
        self._kernel32.OpenProcess.restype = wintypes.HANDLE
        self._kernel32.OpenProcess.argtypes = [
            wintypes.DWORD,
            wintypes.BOOL,
            wintypes.DWORD,
        ]
        self._kernel32.CloseHandle.argtypes = [wintypes.HANDLE]
        self._kernel32.CreateToolhelp32Snapshot.restype = wintypes.HANDLE
        self._kernel32.CreateToolhelp32Snapshot.argtypes = [wintypes.DWORD, wintypes.DWORD]
        self._kernel32.Process32FirstW.argtypes = [wintypes.HANDLE, ctypes.c_void_p]
        self._kernel32.Process32NextW.argtypes = [wintypes.HANDLE, ctypes.c_void_p]
        self._psapi.GetProcessMemoryInfo.argtypes = [
            wintypes.HANDLE,
            ctypes.c_void_p,
            wintypes.DWORD,
        ]
        self._handles: dict[int, int] = {}
        self._open(root_pid)
        self.discover_children()

    def _open(self, pid: int) -> None:
        if pid in self._handles:
            return
        access = (
            self._PROCESS_QUERY_INFORMATION
            | self._PROCESS_QUERY_LIMITED
            | self._PROCESS_VM_READ
        )
        handle = self._kernel32.OpenProcess(access, False, pid)
        if handle:
            self._handles[pid] = handle

    def discover_children(self) -> None:
        ctypes = self._ctypes
        wintypes = self._wintypes

        class PROCESSENTRY32W(ctypes.Structure):
            _fields_ = [
                ("dwSize", wintypes.DWORD),
                ("cntUsage", wintypes.DWORD),
                ("th32ProcessID", wintypes.DWORD),
                ("th32DefaultHeapID", ctypes.POINTER(ctypes.c_ulong)),
                ("th32ModuleID", wintypes.DWORD),
                ("cntThreads", wintypes.DWORD),
                ("th32ParentProcessID", wintypes.DWORD),
                ("pcPriClassBase", ctypes.c_long),
                ("dwFlags", wintypes.DWORD),
                ("szExeFile", wintypes.WCHAR * 260),
            ]

        invalid = wintypes.HANDLE(-1).value
        snapshot = self._kernel32.CreateToolhelp32Snapshot(self._TH32CS_SNAPPROCESS, 0)
        if snapshot == invalid:
            return
        children: dict[int, list[int]] = {}
        entry = PROCESSENTRY32W()
        entry.dwSize = ctypes.sizeof(PROCESSENTRY32W)
        more = self._kernel32.Process32FirstW(snapshot, ctypes.byref(entry))
        while more:
            children.setdefault(int(entry.th32ParentProcessID), []).append(
                int(entry.th32ProcessID)
            )
            more = self._kernel32.Process32NextW(snapshot, ctypes.byref(entry))
        self._kernel32.CloseHandle(snapshot)
        pending = list(self._handles)
        seen = set(pending)
        i = 0
        while i < len(pending):
            for child in children.get(pending[i], []):
                if child not in seen:
                    seen.add(child)
                    pending.append(child)
                    self._open(child)
            i += 1

    def current_kib(self) -> int:
        ctypes = self._ctypes
        wintypes = self._wintypes

        class PROCESS_MEMORY_COUNTERS(ctypes.Structure):
            _fields_ = [
                ("cb", wintypes.DWORD),
                ("PageFaultCount", wintypes.DWORD),
                ("PeakWorkingSetSize", ctypes.c_size_t),
                ("WorkingSetSize", ctypes.c_size_t),
                ("QuotaPeakPagedPoolUsage", ctypes.c_size_t),
                ("QuotaPagedPoolUsage", ctypes.c_size_t),
                ("QuotaPeakNonPagedPoolUsage", ctypes.c_size_t),
                ("QuotaNonPagedPoolUsage", ctypes.c_size_t),
                ("PagefileUsage", ctypes.c_size_t),
                ("PeakPagefileUsage", ctypes.c_size_t),
            ]

        total = 0
        for handle in self._handles.values():
            counters = PROCESS_MEMORY_COUNTERS()
            counters.cb = ctypes.sizeof(PROCESS_MEMORY_COUNTERS)
            ok = self._psapi.GetProcessMemoryInfo(
                handle, ctypes.byref(counters), counters.cb
            )
            if ok:
                total += int(counters.WorkingSetSize) // 1024
        return total

    def close(self) -> None:
        for handle in self._handles.values():
            self._kernel32.CloseHandle(handle)
        self._handles.clear()


def linux_tree_rss_kib(root_pid: int) -> int:
    pids = [root_pid]
    seen = {root_pid}
    i = 0
    while i < len(pids):
        pid = pids[i]
        children_file = Path(f"/proc/{pid}/task/{pid}/children")
        try:
            text = children_file.read_text(encoding="utf-8")
        except OSError:
            text = ""
        for token in text.split():
            child = int(token)
            if child not in seen:
                seen.add(child)
                pids.append(child)
        i += 1

    total = 0
    for pid in pids:
        status = Path(f"/proc/{pid}/status")
        try:
            lines = status.read_text(encoding="utf-8").splitlines()
        except OSError:
            continue
        for line in lines:
            if line.startswith("VmRSS:"):
                total += int(line.split()[1])
                break
    return total


def darwin_tree_rss_kib(root_pid: int) -> int:
    try:
        completed = subprocess.run(
            ["ps", "-axo", "pid=,ppid=,rss="],
            check=False,
            capture_output=True,
            text=True,
        )
    except OSError:
        return 0
    processes: dict[int, tuple[int, int]] = {}
    for line in completed.stdout.splitlines():
        fields = line.split()
        if len(fields) == 3:
            processes[int(fields[0])] = (int(fields[1]), int(fields[2]))
    descendants = {root_pid}
    changed = True
    while changed:
        changed = False
        for pid, (parent, _rss) in processes.items():
            if parent in descendants and pid not in descendants:
                descendants.add(pid)
                changed = True
    return sum(processes.get(pid, (0, 0))[1] for pid in descendants)


def rss_sampler() -> Callable[[int], int]:
    system = platform.system()
    if system == "Linux":
        return linux_tree_rss_kib
    if system == "Darwin":
        return darwin_tree_rss_kib
    raise SystemExit(f"unsupported platform for RSS sampling: {system}")


def measure(command: list[str], *, cwd: Path, env: dict[str, str]) -> tuple[int, int]:
    start = time.perf_counter()
    proc = subprocess.Popen(
        command,
        cwd=cwd,
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.PIPE,
    )
    tracker: _WindowsRssTracker | None = None
    sample_rss: Callable[[int], int] | None = None
    if platform.system() == "Windows":
        tracker = _WindowsRssTracker(proc.pid)
    else:
        sample_rss = rss_sampler()
    peak = 0
    try:
        while True:
            if tracker is not None:
                peak = max(peak, tracker.current_kib())
                tracker.discover_children()
                peak = max(peak, tracker.current_kib())
            elif sample_rss is not None:
                try:
                    peak = max(peak, sample_rss(proc.pid))
                except OSError:
                    pass
            if proc.poll() is not None:
                break
            time.sleep(0.002)
        if tracker is not None:
            peak = max(peak, tracker.current_kib())
        elif sample_rss is not None:
            try:
                peak = max(peak, sample_rss(proc.pid))
            except OSError:
                pass
    finally:
        stderr = proc.stderr.read() if proc.stderr is not None else b""
        if proc.returncode is None:
            proc.wait()
        if tracker is not None:
            tracker.close()
    wall_ms = int(round((time.perf_counter() - start) * 1000))
    if proc.returncode != 0:
        detail = stderr.decode("utf-8", errors="replace")
        raise SystemExit(
            f"validator sample failed ({proc.returncode}): {command}\n{detail}"
        )
    return wall_ms, peak

def production_go_modules(root: Path) -> list[str]:
    go_mod = (root / "go.mod").read_text(encoding="utf-8")
    modules: list[str] = []
    in_require = False
    for raw in go_mod.splitlines():
        line = raw.split("//", 1)[0].strip()
        if line == "require (":
            in_require = True
            continue
        if in_require:
            if line == ")":
                in_require = False
            elif line:
                modules.append(line.replace(" ", "@"))
            continue
        if line.startswith("require ") and not line.endswith("("):
            modules.append(line[len("require ") :].replace(" ", "@"))
    return modules


def pending_ci(source: str) -> dict[str, object]:
    return {
        "samples": [],
        "p50": 0,
        "p95": 0,
        "n": 0,
        "source": source,
    }


def collect(root: Path, n: int) -> dict[str, object]:
    if n < 5:
        raise SystemExit("n must be >= 5")
    fixture = root / FIXTURE
    if not fixture.is_file():
        raise SystemExit(f"missing fixture: {fixture}")

    uv = shutil.which("uv")
    if uv is None:
        raise SystemExit("uv is required for Python samples")
    python_cmd = [
        uv,
        "run",
        "--project",
        str(root / "actions" / "repository-policy"),
        "--locked",
        "python",
        str(root / "actions" / "repository-policy" / "validate.py"),
        "--fixture",
        str(fixture),
        "--check",
        "all",
    ]

    env = os.environ.copy()
    env["CGO_ENABLED"] = "0"
    env.pop("GITHUB_STEP_SUMMARY", None)

    go = shutil.which("go")
    if go is None:
        raise SystemExit("go is required for Go samples")

    with tempfile.TemporaryDirectory(prefix="repo-ops-bench-") as tmp:
        binary_name = "repo-ops-validator.exe" if os.name == "nt" else "repo-ops-validator"
        binary = Path(tmp) / binary_name
        run_checked(
            [go, "build", "-o", str(binary), "./cmd/repo-ops-validator"],
            cwd=root,
            env=env,
        )
        go_cmd = [str(binary), "--fixture", str(fixture), "--check", "all"]

        # Discard one warmup per language so venv/binary page-in is not sample 1.
        measure(python_cmd, cwd=root, env=env)
        measure(go_cmd, cwd=root, env=env)

        python_wall: list[int] = []
        python_rss: list[int] = []
        go_wall: list[int] = []
        go_rss: list[int] = []
        for _ in range(n):
            wall, rss = measure(python_cmd, cwd=root, env=env)
            python_wall.append(wall)
            python_rss.append(rss)
            wall, rss = measure(go_cmd, cwd=root, env=env)
            go_wall.append(wall)
            go_rss.append(rss)

    head = subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=root,
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()
    dirty = subprocess.run(
        ["git", "status", "--porcelain"],
        cwd=root,
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()

    oracle_commit = "654d81f68ee1db5baf33c89016d3ddb659e98220"
    metadata = root / "tests" / "testdata" / "go-oracle" / "metadata.json"
    if metadata.is_file():
        oracle_commit = json.loads(metadata.read_text(encoding="utf-8"))["oracle_commit"]

    go_modules = production_go_modules(root)
    explanations = [
        "Percentiles use nearest-rank ceil(p*n) on the sorted samples, 1-based, clamped to [1, n].",
        "local_cold_start_ms is wall time of a fresh process `--fixture fixtures/valid-high.json --check all` after one discarded warmup per language. Go is a CGO_ENABLED=0 prebuilt binary; Python is `uv run --project actions/repository-policy --locked python actions/repository-policy/validate.py`. Compiler/uv bootstrap time is excluded.",
        "peak_rss_kib is the maximum sampled aggregate current resident set of the process tree in KiB. Windows uses WorkingSetSize while retaining child handles; Linux uses VmRSS; macOS enumerates descendants from ps pid/ppid/rss. This is not ru_maxrss and is not comparable to GitHub runner memory.",
        f"Local host: {platform.platform()} python={platform.python_version()} go={subprocess.run([go, 'version'], capture_output=True, text=True, check=True).stdout.strip()} uv={subprocess.run([uv, '--version'], capture_output=True, text=True, check=True).stdout.strip()}. OS file cache was not dropped.",
        "Local Windows/macOS numbers are not GitHub ubuntu-latest clean-CI samples.",
        "clean_ci_wall_ms.python is pending: requires n>=5 empty-cache production `quality` job durations after this branch is on GitHub. Not fabricated.",
        "clean_ci_wall_ms.go is pending: requires n>=5 empty-cache runs of `.github/workflows/5-go-validator-benchmark.yml` (`go test ./...`). Local `go test ./...` is not a clean-CI sample. Not fabricated.",
        "workflow_steps counts explicit `.github/workflows/policy.yml` job `publish` steps in the Stage 2 tree (8). Proposed Stage 3 keeps those 8 steps and swaps composite internals to build+exec the base-owned Go binary with the same `--event`/`--check` flags. The benchmark-only job is not counted.",
        "production_dependencies.python are the plan's trusted-path entries. production_dependencies.go lists direct require directives in go.mod only (not test-only or benchmark tooling). Current inventory: "
        + (", ".join(go_modules) if go_modules else "stdlib-only")
        + ".",
        f"go_commit is HEAD {head}"
        + ("; worktree had uncommitted Stage 2 files during measurement." if dirty else "."),
        "Benchmarks are evidence only and do not authorize retaining Python after Stage 3.",
    ]

    return {
        "oracle_commit": oracle_commit,
        "go_commit": head,
        "local_cold_start_ms": {
            "python": summarize(python_wall),
            "go": summarize(go_wall),
        },
        "clean_ci_wall_ms": {
            "python": pending_ci("production quality job"),
            "go": pending_ci("5-go-validator-benchmark.yml"),
        },
        "peak_rss_kib": {
            "python": summarize(python_rss),
            "go": summarize(go_rss),
        },
        "workflow_steps": {"python": 8, "go": 8},
        "production_dependencies": {
            "python": ["jsonschema==4.25.1", "pyyaml==6.0.2", "uv"],
            "go": go_modules,
        },
        "explanations": explanations,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--n", type=int, default=N_DEFAULT)
    parser.add_argument("--root", type=Path, default=None)
    parser.add_argument(
        "--write",
        type=Path,
        default=None,
        help="optional local-only output path; canonical Stage 2 evidence is protected",
    )
    args = parser.parse_args()
    root = repo_root_from(args.root.resolve() if args.root else Path.cwd())
    destination = args.write.resolve() if args.write is not None else None
    if destination is not None:
        canonical = (root / "tests" / "testdata" / "benchmarks" / RECORD_NAME).resolve()
        if destination == canonical:
            parser.error("local-only collector cannot overwrite canonical Stage 2 evidence")
    record = collect(root, args.n)
    if destination is not None:
        destination.parent.mkdir(parents=True, exist_ok=True)
        destination.write_text(json.dumps(record, indent=2) + "\n", encoding="utf-8")
    sys.stdout.write(json.dumps(record, indent=2) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
