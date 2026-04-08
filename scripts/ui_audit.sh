#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTDIR="$ROOT/artifacts/ui-audit"
JSON_OUT="$OUTDIR/go-test.json"
SUMMARY_OUT="$OUTDIR/summary.md"

mkdir -p "$OUTDIR"

pushd "$ROOT" >/dev/null

go test -count=1 -json ./app -run 'TestAutomatedUI|TestUIAudit|TestUserJourney' | tee "$JSON_OUT"

python3 - "$JSON_OUT" "$SUMMARY_OUT" <<'PY'
import json
import sys
from collections import defaultdict

json_path, summary_path = sys.argv[1], sys.argv[2]
interesting = ("TestAutomatedUI", "TestUIAudit", "TestUserJourney")
results = defaultdict(lambda: {"status": "unknown", "elapsed": None})

with open(json_path, "r", encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        test = event.get("Test")
        if not test or not test.startswith(interesting):
            continue
        action = event.get("Action")
        if action in {"pass", "fail", "skip"}:
            results[test]["status"] = action
            results[test]["elapsed"] = event.get("Elapsed")

passed = [name for name, data in results.items() if data["status"] == "pass"]
failed = [name for name, data in results.items() if data["status"] == "fail"]
skipped = [name for name, data in results.items() if data["status"] == "skip"]

lines = [
    "# UI Audit Summary",
    "",
    f"- Total scenarios: {len(results)}",
    f"- Passed: {len(passed)}",
    f"- Failed: {len(failed)}",
    f"- Skipped: {len(skipped)}",
    "",
    "## Scenario Results",
    "",
]

for name in sorted(results):
    data = results[name]
    elapsed = data['elapsed']
    elapsed_str = f" ({elapsed:.3f}s)" if isinstance(elapsed, (int, float)) else ""
    lines.append(f"- `{name}`: **{data['status']}**{elapsed_str}")

with open(summary_path, "w", encoding="utf-8") as out:
    out.write("\n".join(lines) + "\n")
PY

popd >/dev/null

echo "ui audit artifacts written to $OUTDIR"
