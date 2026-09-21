#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf '::warning title=Repository policy::%s\n' "$1"
  exit 1
}

command -v jq >/dev/null 2>&1 || fail "jq is required to validate the artifact pin"
command -v node >/dev/null 2>&1 || fail "node is required to download the validator artifact"
command -v sha256sum >/dev/null 2>&1 || fail "sha256sum is required to verify the validator artifact"

: "${GITHUB_ACTION_PATH:?GITHUB_ACTION_PATH is required}"
: "${GITHUB_EVENT_PATH:?GITHUB_EVENT_PATH is required}"
: "${INPUT_CHECK:?INPUT_CHECK is required}"
: "${RUNNER_TEMP:?RUNNER_TEMP is required}"

pin_path="${GITHUB_ACTION_PATH}/artifact.json"
[[ -f "$pin_path" ]] || fail "validator artifact pin is missing"

pin_filter='type == "object"
  and (keys | sort) == (["cgo_enabled","go_mod_sha256","go_sum_sha256","goarch","goos","recipe","sha256","size","source_commit","toolchain","uri"] | sort)
  and (.source_commit | type == "string" and test("^[0-9a-f]{40}$"))
  and (.toolchain | type == "string" and length > 0)
  and .goos == "linux"
  and .goarch == "amd64"
  and .cgo_enabled == "0"
  and (.go_mod_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and (.go_sum_sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and .recipe == "CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o repo-ops-validator ./cmd/repo-ops-validator"
  and (.sha256 | type == "string" and test("^sha256:[0-9a-f]{64}$"))
  and (.size | type == "number" and . > 0 and floor == .)
  and (.uri | type == "string" and test("^https://"))'

jq -e "$pin_filter" "$pin_path" >/dev/null 2>&1 || fail "validator artifact pin is invalid"

read_pin() {
  jq -er "$1" "$pin_path" 2>/dev/null
}

expected_mod="$(read_pin '.go_mod_sha256')" || fail "validator artifact pin is unreadable"
expected_sum="$(read_pin '.go_sum_sha256')" || fail "validator artifact pin is unreadable"
expected_sha="$(read_pin '.sha256')" || fail "validator artifact pin is unreadable"
expected_size="$(read_pin '.size | tostring')" || fail "validator artifact pin is unreadable"
uri="$(read_pin '.uri')" || fail "validator artifact pin is unreadable"

contract_root="$(cd "${GITHUB_ACTION_PATH}/../.." && pwd -P)" || fail "validator contract root is unavailable"
[[ "sha256:$(sha256sum "${contract_root}/go.mod" | cut -d' ' -f1)" == "$expected_mod" ]] || fail "go.mod does not match the validator artifact pin"
[[ "sha256:$(sha256sum "${contract_root}/go.sum" | cut -d' ' -f1)" == "$expected_sum" ]] || fail "go.sum does not match the validator artifact pin"

work_dir="$(mktemp -d "${RUNNER_TEMP}/repo-ops-validator.XXXXXX")" || fail "validator temporary directory could not be created"
trap 'rm -rf "$work_dir"' EXIT
binary="${work_dir}/repo-ops-validator"

node "${GITHUB_ACTION_PATH}/download.mjs" "$uri" "$expected_size" "$binary" || fail "validator artifact download failed"
[[ -f "$binary" ]] || fail "validator artifact download produced no file"
[[ "$(stat -c '%s' "$binary")" == "$expected_size" ]] || fail "validator artifact length does not match the pin"
[[ "sha256:$(sha256sum "$binary" | cut -d' ' -f1)" == "$expected_sha" ]] || fail "validator artifact digest does not match the pin"
chmod 0500 "$binary"

set +e
REPO_OPS_CONTRACT_ROOT="$contract_root" "$binary" --event "$GITHUB_EVENT_PATH" --check "$INPUT_CHECK"
status=$?
set -e
exit "$status"
