#!/usr/bin/env bash
# Create a large tree so live progress is visible for several seconds in demos.
# Default target: ~2000 text files / ~24MB so `tcount -d` takes ~2–4s on a laptop.
set -euo pipefail
root="${1:-/tmp/tcount-progress-demo}"
files="${2:-2000}"
rm -rf "$root"
mkdir -p "$root"
python3 - "$root" "$files" <<'PY'
import sys
from pathlib import Path

root = Path(sys.argv[1])
n = int(sys.argv[2])
# ~12KB per file → enough BPE work that a full tree takes multiple seconds.
chunk = ("package demo\n// " + ("word " * 20) + "\n") * 80
body_tail = 'func F() string { return "hello world token count" }\n' * 50
for i in range(n):
    d = root / f"pkg{i % 40}" / f"sub{i % 10}"
    d.mkdir(parents=True, exist_ok=True)
    text = chunk + f"const N = {i}\n" + body_tail
    (d / f"f{i:04d}.go").write_text(text)
paths = list(root.rglob("*.go"))
size = sum(p.stat().st_size for p in paths)
print(f"{len(paths)} files, {size / 1e6:.1f} MB in {root}")
PY
