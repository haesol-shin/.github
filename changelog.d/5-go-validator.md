## Added
- Add a Go `repo-ops-validator` CLI with fixture evaluation, live event collection, frozen intent and plan digest vectors, and Go-only semantic parity coverage.
- Record reproducible artifact provenance, isolated benchmark evidence, the production Python baseline, and zero-divergence live shadow evidence before cutover.

## Changed
- Add live GitHub event and external-git state collection, bounded live git collection, and a Go-native changelog folding command.
- Cut production repository-policy evaluation and CI quality over to the immutable, checksum-pinned Go validator; remove uv, the Python validator and changelog implementation, and Python-dependent differential, replay, and benchmark harnesses.
