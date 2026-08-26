#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "=== Populating embedded web assets ==="
# go:embed cannot reference paths above the package (../../frontend),
# so we copy frontend/ into server/webassets/ before building.
cp -r frontend/. server/webassets/

echo "=== Building qo-init C helper ==="
gcc -O2 -o "$ROOT/deploy/qo-init" "$ROOT/deploy/qo-init.c" -lseccomp

echo "=== Building qo binary (statically linked for sandbox init) ==="
cd "$ROOT/qo"
CGO_ENABLED=0 go build -o "$ROOT/deploy/qo" .

echo "=== Building server binary ==="
cd "$ROOT/server"
CGO_ENABLED=0 go build -o "$ROOT/deploy/server" .

echo "=== Staleness guard: binaries must be newer than all sources ==="
check_fresh() {
    local bin="$1" dir="$2"
    local newer
    newer=$(find "$dir" -name '*.go' -newer "$bin" -print -quit 2>/dev/null || true)
    if [ -n "$newer" ]; then
        echo "ERROR: $bin is older than source file: $newer" >&2
        exit 1
    fi
}
check_fresh "$ROOT/deploy/qo"     "$ROOT/qo"
check_fresh "$ROOT/deploy/server" "$ROOT/server"

echo "=== Build complete ==="
echo ""
echo "Deploy these to the event machine:"
echo "  deploy/server        — the Go server binary (contains all frontend assets)"
echo "  deploy/qo            — the qo CLI binary"
echo "  <challenge>.enc      — your encrypted challenge archive(s)"
echo ""
echo "No frontend/ directory needed alongside the binary."
