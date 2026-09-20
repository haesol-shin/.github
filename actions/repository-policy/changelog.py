from __future__ import annotations

import argparse
from collections import defaultdict
import datetime as dt
import os
from pathlib import Path
import re
import subprocess
import tempfile
from typing import Iterable

ALLOWED_SECTIONS = ("Added", "Changed", "Deprecated", "Removed", "Fixed", "Security")
_SECTION_PATTERN = re.compile(r"^## (?P<section>Added|Changed|Deprecated|Removed|Fixed|Security)$")
_VERSION_HEADING_PATTERN = re.compile(
    r"^## (?:\[)?(?P<version>v[0-9]+\.[0-9]+\.[0-9]+)(?:\])?(?: - .*)?$"
)
_FRAGMENT_NAME_PATTERN = re.compile(
    r"^(?:[1-9][0-9]*|direct)-[a-z0-9]+(?:-[a-z0-9]+)*\.md$"
)


class ChangelogError(ValueError):
    """Raised when a fragment set cannot be safely validated or folded."""


def _normalized_lines(text: str) -> list[str]:
    return text.replace("\r\n", "\n").replace("\r", "\n").split("\n")


def parse_fragment(text: str, *, filename: str = "<fragment>") -> dict[str, list[str]]:
    """Parse one strict fragment into section -> bullet lines."""
    lines = _normalized_lines(text)
    while lines and not lines[-1].strip():
        lines.pop()
    if not lines or not any(line.strip() for line in lines):
        raise ChangelogError(f"{filename}: fragment is empty")

    sections: dict[str, list[str]] = {}
    current: str | None = None
    for number, line in enumerate(lines, 1):
        if not line.strip():
            continue
        heading = _SECTION_PATTERN.fullmatch(line)
        if heading:
            current = heading.group("section")
            if current in sections:
                raise ChangelogError(f"{filename}:{number}: duplicate {current} heading")
            sections[current] = []
            continue
        if current is None:
            raise ChangelogError(
                f"{filename}:{number}: content must follow an allowed section heading"
            )
        if not line.startswith("- ") or not line[2:].strip():
            raise ChangelogError(
                f"{filename}:{number}: each fragment entry must be a non-empty `- ` bullet"
            )
        sections[current].append(line.rstrip())

    if not sections:
        raise ChangelogError(f"{filename}: fragment has no allowed section heading")
    empty = [section for section, bullets in sections.items() if not bullets]
    if empty:
        raise ChangelogError(
            f"{filename}: section(s) have no non-empty bullet: {', '.join(empty)}"
        )
    return sections


def fragment_paths(root: Path) -> list[Path]:
    if not root.is_dir():
        raise ChangelogError(f"fragment root does not exist: {root}")
    paths = [path for path in root.iterdir() if path.is_file() and path.suffix == ".md"]
    return sorted((path for path in paths if path.name != "README.md"), key=lambda path: path.name)


def read_fragments(root: Path) -> list[tuple[Path, dict[str, list[str]]]]:
    paths = fragment_paths(root)
    if not paths:
        raise ChangelogError(f"fragment root contains no fragments: {root}")
    result: list[tuple[Path, dict[str, list[str]]]] = []
    for path in paths:
        if _FRAGMENT_NAME_PATTERN.fullmatch(path.name) is None:
            raise ChangelogError(f"{path.name}: invalid fragment filename")
        result.append(
            (path, parse_fragment(path.read_text(encoding="utf-8"), filename=path.name))
        )
    return result


def _check_version(version: str) -> None:
    if not _VERSION_PATTERN.fullmatch(version):
        raise ChangelogError("version must match vX.Y.Z")


def _check_date(date: str) -> None:
    try:
        dt.date.fromisoformat(date)
    except ValueError as error:
        raise ChangelogError("date must be an ISO-8601 calendar date") from error


def _assert_changelog_clean(path: Path) -> None:
    if not path.exists():
        return
    try:
        repository = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            cwd=path.parent,
            check=False,
            capture_output=True,
            text=True,
        )
    except OSError:
        return
    if repository.returncode:
        return
    root = Path(repository.stdout.strip())
    try:
        relative = path.resolve().relative_to(root.resolve()).as_posix()
    except ValueError:
        return
    for args in (("diff", "--quiet", "--", relative), ("diff", "--cached", "--quiet", "--", relative)):
        result = subprocess.run(["git", *args], cwd=root, check=False)
        if result.returncode:
            raise ChangelogError(f"changelog is dirty: {relative}")


def _existing_versions(text: str) -> set[str]:
    return {
        match.group("version")
        for line in _normalized_lines(text)
        if (match := _VERSION_HEADING_PATTERN.fullmatch(line))
    }


def render_release(
    fragments: Iterable[tuple[Path, dict[str, list[str]]]],
    *,
    version: str,
    date: str,
) -> str:
    grouped: dict[str, list[str]] = defaultdict(list)
    for _path, sections in fragments:
        for section in ALLOWED_SECTIONS:
            grouped[section].extend(sections.get(section, []))
    lines = [f"## [{version}] - {date}", ""]
    for section in ALLOWED_SECTIONS:
        bullets = grouped[section]
        if not bullets:
            continue
        lines.extend((f"### {section}", *bullets, ""))
    return "\n".join(lines).rstrip() + "\n"


def fold_changelog(
    root: Path,
    changelog: Path,
    *,
    version: str,
    date: str,
) -> list[Path]:
    """Validate and consume every fragment exactly once, returning consumed paths."""
    _check_version(version)
    _check_date(date)
    fragments = read_fragments(root)
    existing = changelog.read_text(encoding="utf-8") if changelog.exists() else ""
    if version in _existing_versions(existing):
        raise ChangelogError(f"changelog already contains {version}")
    _assert_changelog_clean(changelog)

    generated = render_release(fragments, version=version, date=date)
    if existing and not existing.endswith("\n"):
        existing += "\n"
    if existing and not existing.endswith("\n\n"):
        existing += "\n"
    content = existing + generated
    changelog.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        "w", encoding="utf-8", dir=changelog.parent, prefix=f".{changelog.name}.", delete=False
    ) as temporary:
        temporary.write(content)
        temporary_path = Path(temporary.name)
    try:
        os.replace(temporary_path, changelog)
    except Exception:
        temporary_path.unlink(missing_ok=True)
        raise
    consumed = [path for path, _sections in fragments]
    for path in consumed:
        path.unlink()
    return consumed


def main() -> int:
    parser = argparse.ArgumentParser(description="Validate and fold changelog fragments")
    parser.add_argument("--root", type=Path, default=Path("changelog.d"))
    parser.add_argument("--changelog", type=Path, default=Path("CHANGELOG.md"))
    parser.add_argument("--version", required=True)
    parser.add_argument("--date", required=True)
    args = parser.parse_args()
    try:
        consumed = fold_changelog(
            args.root,
            args.changelog,
            version=args.version,
            date=args.date,
        )
    except ChangelogError as error:
        parser.error(str(error))
    print(f"folded {args.version} from {len(consumed)} fragment(s)")
    for path in consumed:
        print(path.as_posix())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
