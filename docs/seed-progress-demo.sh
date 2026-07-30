#!/usr/bin/env bash
# Create a mid-size tree so live progress paint delay can fire in demos.
set -euo pipefail
root="${1:-/tmp/tcount-progress-demo}"
rm -rf "$root"
mkdir -p "$root"
python3 - "$root" <<'PY'
import sys
from pathlib import Path
root = Path(sys.argv[1])
for i in range(180):
    d = root / f"pkg{i % 12}"
    d.mkdir(parents=True, exist_ok=True)
    body = ("package p\n// line\n" * (20 + i % 40)) + (f'var X{i} = "hello world {i}"\n' * 5)
    (d / f"file_{i:03d}.go").write_text(body)
print(len(list(root.rglob("*.go"))), "files in", root)
PY
