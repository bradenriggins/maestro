#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
UPDATE="${UPDATE_UI_SNAPSHOTS:-0}"

pushd "$ROOT" >/dev/null
UPDATE_UI_SNAPSHOTS="$UPDATE" go test -count=1 -v ./app -run 'TestUISnapshots'
popd >/dev/null
