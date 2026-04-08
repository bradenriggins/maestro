#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

BIN="$TMP/maestro"
HOME_DEBUG="$TMP/home-debug"
HOME_DOCTOR="$TMP/home-doctor"
HOME_WORKERS="$TMP/home-workers"
mkdir -p "$HOME_DEBUG" "$HOME_DOCTOR" "$HOME_WORKERS"

pushd "$ROOT" >/dev/null

go build -o "$BIN" ./

run() {
  local home_dir="$1"
  shift
  HOME="$home_dir" \
  TERM="xterm-256color" \
  "$@"
}

expect_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "$haystack" != *"$needle"* ]]; then
    echo "Expected output to contain: $needle" >&2
    echo "Actual output:" >&2
    echo "$haystack" >&2
    exit 1
  fi
}

version_out="$(run "$HOME_DEBUG" "$BIN" version)"
expect_contains "$version_out" "maestro version"

debug_out="$(run "$HOME_DEBUG" "$BIN" debug)"
expect_contains "$debug_out" "$HOME_DEBUG/.maestro/config.json"

set +e
doctor_out="$(run "$HOME_DOCTOR" "$BIN" doctor 2>&1)"
doctor_status=$?
set -e
if [[ $doctor_status -ne 2 ]]; then
  echo "Expected 'maestro doctor' to exit 2 in isolated smoke env, got $doctor_status" >&2
  echo "$doctor_out" >&2
  exit 1
fi
expect_contains "$doctor_out" "Config: not found"
expect_contains "$doctor_out" "Required binaries:"

set +e
workers_out="$(run "$HOME_WORKERS" "$BIN" workers 2>&1)"
workers_status=$?
set -e
if [[ $workers_status -eq 0 ]]; then
  echo "Expected 'maestro workers' to fail without setup" >&2
  exit 1
fi
expect_contains "$workers_out" "not configured; run 'maestro setup' first"

popd >/dev/null

echo "smoke checks passed"
