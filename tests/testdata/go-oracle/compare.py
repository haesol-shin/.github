from __future__ import annotations

import argparse
import importlib.util
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[3]
ORACLE = Path(__file__).resolve().parent
FIXTURES = ROOT / "fixtures"
CONTRACTS = ROOT / "contracts" / "v0.1.0"
DIGESTS = ORACLE / "digests.json"
METADATA = ORACLE / "metadata.json"
MATRIX = ORACLE / "matrix.json"
SCHEMA_FAILURES = ORACLE / "schema-failures.json"
PROBE = ORACLE / "digest_probe.go"
SCHEMA_PROBE = ORACLE / "schema_probe.go"
VALIDATOR_PY = ROOT / "actions" / "repository-policy" / "validate.py"
ORACLE_COMMIT = "654d81f68ee1db5baf33c89016d3ddb659e98220"
ANNOTATION_PREFIX = "::warning title=Repository policy::"
EXCLUDED = {"changelog-conformance.json", "intent-comment-revocation.json"}
CHECKS = ("all", "contract", "merge-approval")
SECRET_VALUE = "repo-ops-stage1-oracle-token-7f1a9c3e5b8d2f40"
SHA256_A = "sha256:" + "a" * 64
HEX40_A = "a" * 40
SCHEMA_LABELS = {
    "changelog-declaration.schema.json": "changelog declaration",
    "intent.schema.json": "repo-ops.intent.v1",
    "merge-review.schema.json": "merge-review receipt",
    "plan-approval.schema.json": "plan approval",
    "repository-policy.schema.json": "repository policy",
}
SCHEMA_REQUIRED = {
    "changelog-declaration.schema.json": ("record", "kind", "value"),
    "intent.schema.json": ("record", "decision", "risk", "intent", "supersedes"),
    "merge-review.schema.json": (
        "record",
        "verdict",
        "risk",
        "intent",
        "plan",
        "base",
        "head",
        "diff",
        "runtime",
        "model",
    ),
    "plan-approval.schema.json": (
        "record",
        "decision",
        "risk",
        "issue",
        "intent",
        "plan",
        "plan-commit",
    ),
    "repository-policy.schema.json": (
        "contract",
        "profile",
        "quality",
        "review",
        "changelog",
        "release",
    ),
}
SCHEMA_SAMPLES = {
    "changelog-declaration.schema.json": {
        "record": "repo-ops.changelog.v1",
        "kind": "not-required",
        "value": "receipt-validation",
    },
    "intent.schema.json": {
        "record": "repo-ops.intent.v1",
        "decision": "accepted",
        "risk": "low",
        "intent": SHA256_A,
        "supersedes": "none",
    },
    "merge-review.schema.json": {
        "record": "repo-ops.merge-review.v1",
        "verdict": "merge-ready",
        "risk": "low",
        "intent": "none",
        "plan": "none",
        "base": HEX40_A,
        "head": HEX40_A,
        "diff": SHA256_A,
        "runtime": "x",
        "model": "x",
    },
    "plan-approval.schema.json": {
        "record": "repo-ops.plan-approval.v1",
        "decision": "approved",
        "risk": "high",
        "issue": 1,
        "intent": SHA256_A,
        "plan": SHA256_A,
        "plan-commit": HEX40_A,
    },
    "repository-policy.schema.json": {
        "contract": "repo-ops/v0.1.0",
        "profile": "python-engine",
        "quality": {"check": "quality", "commands": ["test"]},
        "review": {"shared_identity": False},
        "changelog": {"mode": "none"},
        "release": {"enabled": False},
    },
}
DIGEST_TEMPLATES = [
    {
        "id": "legacy_intent_high",
        "kind": "canonical",
        "payload": {
            "outcome": "Add a trusted validator without executing pull request code.\n",
            "risk": "high",
        },
    },
    {
        "id": "v1_intent_issue_42_high",
        "kind": "canonical",
        "payload": {
            "issue": 42,
            "outcome": "Add a trusted validator without executing pull request code.\n",
            "risk": "high",
        },
    },
    {
        "id": "v1_intent_isolated_outcome",
        "kind": "canonical",
        "payload": {
            "issue": 42,
            "outcome": "The trusted validator remains isolated from pull request code.\n",
            "risk": "high",
        },
    },
    {
        "id": "plan_trusted_validator",
        "kind": "plan",
        "text": "Add a trusted validator without executing pull request code.",
    },
    {
        "id": "plan_trusted_validator_crlf",
        "kind": "plan",
        "text": "Add a trusted validator without executing pull request code.\r\n",
    },
]


def load_python_validator() -> Any:
    spec = importlib.util.spec_from_file_location("repo_ops_validator", VALIDATOR_PY)
    assert spec and spec.loader
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


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


def write_text(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text, encoding="utf-8", newline="\n")


def write_json(path: Path, payload: Any) -> None:
    write_text(path, json.dumps(payload, indent=2, ensure_ascii=False) + "\n")


def capture(
    cmd: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    dest: Path,
    step_summary: bool = False,
) -> dict[str, Any]:
    dest.mkdir(parents=True, exist_ok=True)
    child_env = env.copy()
    if step_summary:
        child_env["GITHUB_STEP_SUMMARY"] = str(dest / "summary.md")
    else:
        child_env.pop("GITHUB_STEP_SUMMARY", None)
    completed = subprocess.run(
        cmd,
        cwd=cwd,
        env=child_env,
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
    leaked = SECRET_VALUE in stdout or SECRET_VALUE in stderr
    summary = dest / "summary.md"
    if summary.exists() and SECRET_VALUE in summary.read_text(encoding="utf-8", errors="replace"):
        leaked = True
    if leaked:
        raise SystemExit(f"injected secret value leaked under {dest}")
    return payload


def matrix_fixtures() -> list[Path]:
    return sorted(
        path
        for path in FIXTURES.glob("*.json")
        if path.name not in EXCLUDED
    )


def child_env() -> dict[str, str]:
    env = os.environ.copy()
    env["GITHUB_TOKEN"] = SECRET_VALUE
    env["CGO_ENABLED"] = "0"
    return env


def capture_impl(out: Path, impl: str) -> dict[tuple[str, str], dict[str, Any]]:
    cmd = python_cmd() if impl == "python" else go_cmd()
    env = child_env()
    observed: dict[tuple[str, str], dict[str, Any]] = {}
    for fixture in matrix_fixtures():
        for check in CHECKS:
            rel = f"{fixture.stem}/{check}"
            observed[(fixture.name, check)] = capture(
                [*cmd, "--fixture", str(fixture), "--check", check],
                cwd=ROOT,
                env=env,
                dest=out / impl / rel,
            )
            capture(
                [*cmd, "--fixture", str(fixture), "--check", check],
                cwd=ROOT,
                env=env,
                dest=out / "summary-scan" / impl / rel,
                step_summary=True,
            )
    return observed


def matrix_index(cases: list[dict[str, Any]]) -> dict[tuple[str, str], dict[str, Any]]:
    index: dict[tuple[str, str], dict[str, Any]] = {}
    for row in cases:
        index[(row["fixture"], row["check"])] = {
            "exit": row["exit"],
            "findings": row["findings"],
            "check_conclusion": row["check_conclusion"],
        }
    return index


def compare_matrix(out: Path, goldens: dict[tuple[str, str], dict[str, Any]], impls: tuple[str, ...]) -> list[str]:
    errors: list[str] = []
    expected_keys = {(fixture.name, check) for fixture in matrix_fixtures() for check in CHECKS}
    if set(goldens) != expected_keys:
        errors.append("committed semantic matrix keys drifted from fixture × check set")
    for impl in impls:
        observed = capture_impl(out, impl)
        for fixture in matrix_fixtures():
            for check in CHECKS:
                rel = f"{fixture.stem}/{check}"
                expected = goldens.get((fixture.name, check))
                got = observed[(fixture.name, check)]
                if expected is None:
                    errors.append(f"{rel}: missing golden")
                elif got != expected:
                    errors.append(f"{impl} {rel}: {got} != golden {expected}")
    return errors


def python_digest(validator: Any, vector: dict[str, Any]) -> str:
    kind = vector["kind"]
    if kind == "canonical":
        return validator.canonical_digest(vector["payload"])
    if kind == "plan":
        return validator.plan_digest(vector["text"])
    raise SystemExit(f"unknown digest kind {kind}")


def digest_document(validator: Any) -> dict[str, Any]:
    vectors = []
    for template in DIGEST_TEMPLATES:
        vector = dict(template)
        vector["digest"] = python_digest(validator, template)
        vectors.append(vector)
    return {
        "oracle_commit": ORACLE_COMMIT,
        "source": "tests/test_validator.py intent payloads and canonical plan_digest of an exact UTF-8 plan byte string",
        "vectors": vectors,
    }


def compare_python_digests(validator: Any, expected: list[dict[str, Any]]) -> list[str]:
    errors: list[str] = []
    for vector in expected:
        observed = python_digest(validator, vector)
        if observed != vector["digest"]:
            errors.append(
                f"python {vector['id']}: {observed} != {vector['digest']}"
            )
    return errors


def compare_go_digests(expected: list[dict[str, Any]]) -> list[str]:
    completed = subprocess.run(
        ["go", "run", str(PROBE), str(DIGESTS)],
        cwd=ROOT,
        env={**os.environ, "CGO_ENABLED": "0"},
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode != 0:
        return [
            "go digest probe failed: "
            + completed.stderr.decode("utf-8", errors="replace")
        ]
    observed: dict[str, str] = {}
    for line in completed.stdout.decode("utf-8").splitlines():
        if not line.strip():
            continue
        row = json.loads(line)
        observed[row["id"]] = row["digest"]
    errors: list[str] = []
    for vector in expected:
        got = observed.get(vector["id"])
        if got != vector["digest"]:
            errors.append(f"go {vector['id']}: {got} != {vector['digest']}")
    return errors


def pointer(path: Any) -> str:
    if not path:
        return ""
    parts = []
    for item in path:
        parts.append(str(item).replace("~", "~0").replace("/", "~1"))
    return "/" + "/".join(parts)


def identities(schema: dict[str, Any], instance: dict[str, Any]) -> list[dict[str, str]]:
    from jsonschema import Draft202012Validator

    errors = sorted(
        Draft202012Validator(schema).iter_errors(instance),
        key=lambda error: list(error.path),
    )
    return [
        {"instance_path": pointer(error.path), "keyword": error.validator}
        for error in errors
    ]


def frozen_schemas() -> list[Path]:
    return sorted(CONTRACTS.glob("*.schema.json"))


def schema_failure_document() -> dict[str, Any]:
    from jsonschema import Draft202012Validator

    cases = []
    for path in frozen_schemas():
        schema = json.loads(path.read_text(encoding="utf-8"))
        label = SCHEMA_LABELS[path.name]
        sample = SCHEMA_SAMPLES[path.name]
        if not Draft202012Validator(schema).is_valid(sample):
            raise SystemExit(f"schema sample is invalid: {path.name}")
        omissions = []
        required = SCHEMA_REQUIRED[path.name]
        if tuple(schema.get("required") or ()) != required:
            raise SystemExit(f"schema required fields drifted: {path.name}")
        for field in required:
            instance = json.loads(json.dumps(sample))
            del instance[field]
            omissions.append(
                {
                    "omitted": field,
                    "failures": identities(schema, instance),
                }
            )
        cases.append(
            {
                "schema": path.name,
                "schema_id": schema["$id"],
                "label": label,
                "empty_object": identities(schema, {}),
                "required_omissions": omissions,
            }
        )
    return {"oracle_commit": ORACLE_COMMIT, "cases": cases}


def compare_schema_failures(
    committed: dict[str, Any],
    impls: tuple[str, ...],
    out: Path,
) -> list[str]:
    errors: list[str] = []
    expected = schema_failure_document()
    if committed != expected:
        errors.append("schema-failures.json drifted from repaired-base Python identities")
    if "go" not in impls:
        return errors

    rows = []
    expected_go: dict[str, list[dict[str, str]]] = {}
    for case in committed["cases"]:
        schema_name = case["schema"]
        rows.append(
            {
                "id": f"{schema_name}:empty",
                "schema": str(CONTRACTS / schema_name),
                "label": case["label"],
                "instance": {},
            }
        )
        expected_go[f"{schema_name}:empty"] = case["empty_object"]
        sample = SCHEMA_SAMPLES[schema_name]
        for omission in case["required_omissions"]:
            instance = json.loads(json.dumps(sample))
            del instance[omission["omitted"]]
            row_id = f"{schema_name}:omit:{omission['omitted']}"
            rows.append(
                {
                    "id": row_id,
                    "schema": str(CONTRACTS / schema_name),
                    "label": case["label"],
                    "instance": instance,
                }
            )
            expected_go[row_id] = omission["failures"]

    probe_input = out / "schema-probe-input.json"
    write_json(probe_input, {"rows": rows})
    completed = subprocess.run(
        ["go", "run", str(SCHEMA_PROBE), str(probe_input)],
        cwd=ROOT,
        env={**os.environ, "CGO_ENABLED": "0"},
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode != 0:
        errors.append(
            "go schema probe failed: "
            + completed.stderr.decode("utf-8", errors="replace")
        )
        return errors
    observed_go = {
        row["id"]: row["failures"]
        for line in completed.stdout.decode("utf-8").splitlines()
        if line.strip()
        for row in [json.loads(line)]
    }
    if observed_go != expected_go:
        errors.append("Go schema failure identities drifted from frozen goldens")
    return errors


def metadata_document() -> dict[str, Any]:
    return {
        "oracle_commit": ORACLE_COMMIT,
        "stage": 1,
        "checks": list(CHECKS),
        "excluded_fixtures": sorted(EXCLUDED),
        "excluded_reasons": {
            "changelog-conformance.json": "folding-only",
            "intent-comment-revocation.json": "workflow-only",
        },
        "annotation_prefix": ANNOTATION_PREFIX,
        "merge_approval_contract_block": "contract check did not succeed; merge approval is blocked",
        "secret_scan": "fail if the injected environment value appears in captured bytes; do not treat the substring GITHUB_TOKEN as the leak oracle",
        "fixture_goldens": "tests/testdata/go-oracle/matrix.json",
    }


def matrix_document(observed: dict[tuple[str, str], dict[str, Any]]) -> dict[str, Any]:
    cases = []
    for fixture in matrix_fixtures():
        for check in CHECKS:
            payload = observed[(fixture.name, check)]
            cases.append(
                {
                    "fixture": fixture.name,
                    "check": check,
                    "exit": payload["exit"],
                    "findings": payload["findings"],
                    "check_conclusion": payload["check_conclusion"],
                }
            )
    return {"oracle_commit": ORACLE_COMMIT, "cases": cases}


def require_commit(doc: dict[str, Any], path: Path, errors: list[str]) -> None:
    if doc.get("oracle_commit") != ORACLE_COMMIT:
        errors.append(f"{path.name} oracle_commit drifted")


def require_oracle_tree() -> None:
    protected = (
        "actions/repository-policy",
        "contracts/v0.1.0",
        "fixtures",
        "tests/test_validator.py",
    )
    completed = subprocess.run(
        ["git", "diff", "--quiet", ORACLE_COMMIT, "--", *protected],
        cwd=ROOT,
        check=False,
    )
    if completed.returncode == 1:
        raise SystemExit(
            "refusing regeneration: Python oracle inputs differ from "
            f"{ORACLE_COMMIT}"
        )
    status = subprocess.run(
        [
            "git",
            "status",
            "--porcelain=v1",
            "--untracked-files=all",
            "--",
            *protected,
        ],
        cwd=ROOT,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if status.returncode != 0:
        raise SystemExit("unable to inspect repaired-base Python oracle inputs")
    if status.stdout:
        raise SystemExit(
            "refusing regeneration: Python oracle inputs contain local changes"
        )
    if completed.returncode != 0:
        raise SystemExit("unable to verify repaired-base Python oracle inputs")



def regenerate(out: Path) -> None:
    require_oracle_tree()
    validator = load_python_validator()
    write_json(DIGESTS, digest_document(validator))
    write_json(SCHEMA_FAILURES, schema_failure_document())
    write_json(MATRIX, matrix_document(capture_impl(out, "python")))
    write_json(METADATA, metadata_document())


def main() -> int:
    parser = argparse.ArgumentParser(description="Stage 1 Python/Go semantic differential")
    parser.add_argument(
        "--out",
        type=Path,
        default=ORACLE / "observed",
        help="directory created for captured stdout/stderr/exit (not compared by bytes)",
    )
    parser.add_argument(
        "--regenerate",
        action="store_true",
        help="freeze committed goldens from the repaired-base Python oracle only",
    )
    parser.add_argument(
        "--impl",
        choices=("python", "go", "both"),
        default="both",
        help="which implementation to compare against committed semantic goldens",
    )
    args = parser.parse_args()
    out = args.out if args.out.is_absolute() else ROOT / args.out
    out.mkdir(parents=True, exist_ok=True)

    if args.regenerate:
        regenerate(out)
        return 0

    metadata = json.loads(METADATA.read_text(encoding="utf-8"))
    digests = json.loads(DIGESTS.read_text(encoding="utf-8"))
    matrix = json.loads(MATRIX.read_text(encoding="utf-8"))
    failures = json.loads(SCHEMA_FAILURES.read_text(encoding="utf-8"))
    errors: list[str] = []
    require_commit(metadata, METADATA, errors)
    require_commit(digests, DIGESTS, errors)
    require_commit(matrix, MATRIX, errors)
    require_commit(failures, SCHEMA_FAILURES, errors)
    if metadata != metadata_document():
        errors.append("metadata.json drifted from harness constants")
    if set(metadata.get("excluded_fixtures") or []) != EXCLUDED:
        errors.append("excluded fixtures drifted")

    validator = load_python_validator()
    expected_vectors = digests["vectors"]
    impls = ("python", "go") if args.impl == "both" else (args.impl,)
    if "python" in impls:
        errors.extend(compare_python_digests(validator, expected_vectors))
    if "go" in impls:
        errors.extend(compare_go_digests(expected_vectors))
    errors.extend(compare_schema_failures(failures, impls, out))
    errors.extend(compare_matrix(out, matrix_index(matrix["cases"]), impls))
    if errors:
        for error in errors:
            print(error, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
