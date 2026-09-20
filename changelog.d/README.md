# Changelog fragments

Pull requests that change user or operator behavior add one Markdown fragment under `changelog.d/`. The repository policy validates fragments without executing pull-request code.

Use the linked Issue number and a lowercase slug in the filename:

```text
<issue>-<slug>.md
```

Direct low-risk pull requests may use `direct-<slug>.md`. An issue-backed pull request MUST NOT use the direct form. `README.md` is policy documentation, not a fragment.

A fragment contains one or more of these exact level-two headings:

- `Added`
- `Changed`
- `Deprecated`
- `Removed`
- `Fixed`
- `Security`

Each heading MUST have at least one non-empty `- ` bullet. Keep bullets self-contained and do not add prose outside the headings. Fragment files are folded in stable filename order when a release pull request declares `repo-ops.changelog.v1 kind:release value:vX.Y.Z`.

Ordinary pull requests MUST NOT edit `CHANGELOG.md` or delete fragments. A release pull request consumes validated fragments and writes the generated version entry.

From the repository root, the trusted release preparer runs:

```text
python actions/repository-policy/changelog.py --version vX.Y.Z --date YYYY-MM-DD
```

The policy workflows publish machine check contexts `contract` and `merge approval`; the central labels render as `Repository policy / contract` and `Repository policy / merge approval`. Contract evaluation covers current obligations and risk authority, while merge approval additionally requires the successful contract result and an exact-head merge receipt. Pull-request approval comment changes trigger both checks again; removed or edited stale approvals are not current authority.
