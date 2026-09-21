# LecturAL delivery orchestration plan

Status: LecturAL PR #30 and repository-operations PRs #6 and #10 are merged. Issue #5 Stage 2 PR #13 is exact-head reviewed and green at `12fc6b65a7b353eba01fe39a17979a06c01902ce`, but remains unmerged pending separate approval. Production remains Python. Stage 3 must choose and prove a trusted production packaging path—preferably an immutable once-built Linux binary pinned by source commit and SHA-256—close the collector byte/disk bound, then perform the clean Go cutover before the `repo-ops/v0.1.0` release candidate.

Remote handoff record: https://github.com/haesol-shin/.github/issues/3#issuecomment-5755051551

## Operating model

This conversation is the main orchestrator. It owns sequencing, accepted intent, risk, plan approval, evidence reconciliation, merge recommendations, support decisions, and release approval requests.

Implementation runs in one separate OMP session per GitHub Issue. Each issue gets one isolated worktree, one short-lived branch, one committed plan, and one pull request. Worker sessions implement, verify, prepare PRs, and produce exact-head review evidence. They do not merge unrelated work, publish releases, or declare platform support. Temporary helpers spawned by the main orchestrator are read-only researchers or reviewers; only the assigned Issue worker may mutate its worktree.

Durable authority lives in GitHub Issues, committed plans, PRs, CI, smoke artifacts, and review receipts—not chat or OMP memory.

This document fixes sequencing and cross-Issue boundaries; it is not a worker execution prompt. Each implementation or release-readiness Issue must commit its own executable plan with pinned external contracts, exact inputs, commands, evidence schema, and acceptance gates before mutation begins.

## Agreed decisions

- PR #30 merged only after user approval. Issue #22 is complete after verification of the exact merged head and post-merge CI.
- Adopt the central repository contract in advisory mode before release preparation.
- Use squash merge, Conventional Commit PR titles, and per-change `changelog.d/` fragments.
- Keep the `py3-none-any` wheel; prove native runtime support with OS-specific end-to-end smoke evidence.
- Do not delay v0.2.0 for the internal architecture refactor or docs reorganization.
- Do not create a tag, GitHub Release, or PyPI publication without separate explicit approval.
- Keep PR #30 scoped to Issue #22; do not add the architecture refactor to the existing XL change.
- Treat the post-v0.2.0 architecture in this plan as a proposal, not an approved implementation contract. Before opening refactor Issues, discuss and obtain user approval for pipeline ownership, protocol and factory boundaries, module layout, migration order, and any public extension surface.

## Current GitHub graph

```text
#19 extraction JSON contract
 └─ PR #20 merged

#21 reproducible benchmark
 └─ PR #24 merged

#22 visual evidence efficiency and reliability
 └─ PR #30 merged; #22 closed
     └─ #29 alignment reuse optimization

#23 caption usability and ASR fallback
 ├─ depends on #19 and #21
 └─ implementation waits for the post-v0.2.0 architecture boundary
```

PR #30 changes 310 files, mostly committed calibration and regression corpora. Its significant code additions include `lectural/alignment.py`, calibration tooling, visual deduplication changes, OCR engine reuse, and benchmark observability. The architecture work remains a separate Issue and PR.

## Delivery sequence

### 1. Verify and merge Issue #22

PR #30 merged as `3c1145b65d5669ca4b5417aea4a177d5c0c6b26b` after the user's decision. Its exact merged head passed Windows, Ubuntu, macOS, and plugin-validation CI, and the merge closed Issue #22.

### 2. Publish the central contract, then adopt repository operations

Complete and release `repo-ops/v0.1.0` in `haesol-shin/.github` before LecturAL adoption. Bootstrap PR #4, contract/fixture PR #6, and Issue #5 Stage 1 PR #10 are merged. Stage 2 PR #13 is reviewed and green but requires separate merge approval. After it merges, commit and approve the Stage 3 plan: define a trusted production packaging path, close the aggregate git-fetch byte/disk bound, re-prove compatibility and security against that path, then perform one clean Python-to-Go cutover. Revise and re-approve the Issue #3 release plan against the exact post-cutover state, prove the candidate through a non-merged LecturAL integration PR, and obtain explicit release approval before tag protection or publication. After the immutable central release exists, open a dedicated high-risk LecturAL adoption Issue in another OMP session.

Deliver:

- `.github/repo-policy.yml` in advisory mode
- central policy caller pinned to an immutable commit
- replacement of the legacy PR checker
- aligned PR and Issue templates
- aggregate `CI / quality`
- aligned contribution and agent rules
- changelog fragment validation and release folding
- GitHub Actions pinned to immutable commits
- exact-head merge-review receipt
- protected `main` requiring both stable checks: `CI / quality` and `Repository policy / contract`

Automatic central validation of LecturAL begins after the adoption PR merges.

### 3. Prepare and audit v0.2.0

Open a separate high-risk coordinating Issue. Audit before editing, report blockers, then create issue-backed PRs only for confirmed minimum changes.

Verify:

- final `main` versus PyPI 0.1.2 behavior and description
- README, CHANGELOG, `pyproject.toml`, plugin manifest, and CLI version agreement
- wheel and sdist build
- clean installation with the `[run]` extra
- `lectural --version --json`
- actual local-video extraction
- GitHub Release workflow and PyPI publishing path
- GitHub Actions OIDC and PyPI Trusted Publishing without long-lived PyPI tokens
- `agent-skills/bundle.toml` LecturAL contract and release pins

Build the final candidate from an exact commit and bind all packaging and smoke evidence to that commit.

### 4. Verify platform support

| Environment | Support target | Required evidence |
| --- | --- | --- |
| Windows 11 x86_64 | Official support | Full runtime smoke |
| Debian GNU/Linux 13 (trixie) ARM64/aarch64 | Official support | Full runtime smoke |
| Ubuntu x86_64 | CI tested | Offline unit and contract tests |
| macOS | CI tested | Offline unit and contract tests |

`pi-server` is an internal hostname, not a public support-platform name.

A full runtime smoke covers isolated installation with `[run]`, `lectural doctor --json`, local video input, faster-whisper STT, frame extraction and deduplication, OCR, completeness, `lectural extract ... --json`, and evidence/artifact path validation. Caption-only runs and offline tests do not establish official support.

Record OS, architecture, Python, ffmpeg, faster-whisper, CTranslate2, PaddleOCR, PaddlePaddle, OpenCV, STT model and compute type, wall time, RTF, peak RAM, temporary and final storage, transcript segment count, representative frame count, OCR status, completeness result, and exit code.

The assigned platform-smoke OMP session records every required field in a durable artifact linked from the release-readiness Issue and bound to the exact candidate commit. Official support passes only when installation and every named end-to-end stage complete with exit code zero, `doctor` is ready, completeness passes, required evidence/artifact paths exist and remain contained, and no unverified runtime gap remains. Any failure or missing field blocks that platform's official-support claim until fixed and rerun.

### 5. Release v0.2.0 after approval

After exact-candidate checks pass, record release approval in the LecturAL release-readiness Issue and link it from the release pull request. The approval must name the exact validated commit and already-built artifact checksums before tag push, GitHub Release, or PyPI publication. Publish only those validated artifacts from that commit. Update the `agent-skills` release pin only after the immutable LecturAL release exists, through a separate linked PR.

### 6. Refactor architecture after v0.2.0

Do this before Issue #29, Issue #23, or new CPU/CUDA/XPU/NPU backend work. Preserve public CLI and JSON behavior.

The architecture below is the concrete discussion proposal. Do not convert it into Issues or implementation plans until the user has reviewed the alternatives and approved the boundary.

The target composition is:

```text
CLI
 └─ explicit PipelineFactory
     └─ ExtractionPipeline
         ├─ SpeechBackend
         ├─ OcrBackend
         ├─ visual processing and AlignmentWorker
         └─ StageObserver

BenchmarkRunner
 └─ the same PipelineFactory and ExtractionPipeline
     └─ BenchmarkObserver
```

`ExtractionPipeline` owns the lifecycle of expensive resources. Backend instances own model, device, batching, and cache state. `SpeechBackend`, `OcrBackend`, and `StageObserver` are the only initial protocols. The explicit factory maps supported configuration names to constructors in searchable code. Adding a backend requires its implementation and one factory mapping, but no change to pipeline orchestration.

Organize modules by responsibility:

```text
lectural/
  pipeline/
  speech/
  vision/
  ocr/
  outputs/
  runtime/

lectural_bench/
  cli.py
  fixtures.py
  runner.py
  metrics.py
  resources.py
  reporting.py
```

Do not split files before establishing the dependency boundaries. First move pHash, histogram, and SSIM kernels into a leaf vision module so `visual.py` and `alignment.py` no longer import each other. Then establish the shared pipeline and migrate every caller. `scripts/benchmark.py` and `scripts/perf_smoke.py` become thin entry points rather than owning independent extraction graphs.

Split the work into reviewable Issue units:

1. Break the `visual.py`/`alignment.py` cycle through a pure leaf kernels module.
2. Move `cli._default_processor()` into `ExtractionPipeline`.
3. Add `ExtractionRequest`, `ExtractionResult`, `ArtifactPaths`, and `ExtractionAssessment` dataclasses.
4. Define `SpeechBackend`, `OcrBackend`, and `StageObserver` protocols.
5. Wrap current engines as `FasterWhisperBackend`, `PaddleOcrBackend`, and `TesseractOcrBackend`; preserve PR #30's one-engine-per-extraction behavior while giving the backend explicit ownership.
6. Add the explicit backend factory and keep backend names separate from model names.
7. Route CLI, `lectural_bench`, and performance smoke through the same pipeline; instrumentation observes stages without defining a second stage graph.
8. Replace `_default_processor()`'s draft-coverage → draft-notes → coverage → notes → coverage loop with an explicit assessment step and one final serialization path, preserving public output.
9. Split `lectural_bench` by fixture loading, execution, metrics, resource sampling, and reporting after the common execution path exists.
10. Define real benchmark lifecycle semantics: a cold repetition uses a fresh subprocess and newly constructed pipeline/models; warm repetitions reuse the process and pipeline instance. Record child PID, cache mode, and backend construction counts so the lifecycle is observable. `gc.collect()` alone is not a cold run.

Keep parsing, pHash/SSIM, alignment gate evaluation, coverage, synthesis, Markdown rendering, timestamp/path validation, and benchmark metrics as pure functions unless they acquire real state.

### 7. Reorganize docs with the refactor

This does not block v0.2.0. The move map below is part of the post-release design proposal and requires the same user consultation before execution. The proposal makes `docs/architecture.md` the canonical design source and keeps only short rules and links in CONTRIBUTING and AGENTS.

| Current | Target |
| --- | --- |
| `docs/synthesis_contract.md` | `docs/contracts/synthesis.md` |
| `docs/benchmark_definition.md` | `docs/benchmarks/methodology.md` |
| `docs/reports/*` | `docs/benchmarks/reports/*` |
| `docs/ac_verification.md` | `docs/verification/acceptance-criteria.md` |
| dated smoke/verification docs | `docs/verification/release-evidence/` |

Inventory and update every broken link or reference in the same PR as each move.

### 8. Continue with Issue #23

Start STT backend expansion only after the post-release architecture boundaries are merged and verified.

## Final PM report

1. Issue #22 status and diff verification
2. v0.2.0 release blockers
3. Windows 11 x86_64 full runtime result
4. Debian 13 ARM64 full runtime result
5. Exact OS support table
6. PyPI and GitHub Release changes
7. Post-v0.2.0 architecture work units
8. Docs move map and broken-reference inventory
9. Verification commands and results

## User consultation and decision gates

Consult the user before a plan fixes a material design choice. Present the problem, viable alternatives, recommendation, tradeoffs, and consequence of deferring when a choice affects public CLI or JSON behavior, architecture or ownership boundaries, extension protocols, dependencies, performance or cost, supported platforms, security or operations, release scope, or delivery order. Consult again when execution evidence would change an approved choice, intent, acceptance criterion, or non-goal.

The user explicitly decides: high-risk plan approval, the post-v0.2.0 architecture and documentation boundary before refactor Issues are opened, repository ruleset or required-check changes, PR merge, response to a failed official-platform smoke, PyPI or GitHub permission changes, release publication, and the later advisory-to-enforce transition.

Within an approved intent and plan, workers may choose local implementation details, tests, file-level mechanics, and reversible cleanup that do not cross those boundaries. Uncertainty about whether a choice is material is itself a consultation trigger.

## Current checkpoint

PR #30 is merged and Issue #22 is closed. In `haesol-shin/.github`, PR #6 completed the central contract feature, PR #10 merged the Go fixture evaluator, and PR #13 contains the reviewed Stage 2 collector/replay implementation. PR #13 is ready but not merged; explicit merge approval remains required. Stage 3 has not started. Its approved plan must define trusted binary or cache provenance, close the aggregate git-fetch byte/disk bound, measure the actual production path, and remove Python/uv in one clean cutover. Issue #3 release-candidate work, the non-merged LecturAL integration gate, tag protection, and `repo-ops/v0.1.0` publication follow that cutover. LecturAL v0.2.0 release readiness and platform smokes follow the immutable central release. The post-v0.2.0 architecture and documentation proposals still require user consultation before refactor Issues, Issue #29, or Issue #23 begin.
