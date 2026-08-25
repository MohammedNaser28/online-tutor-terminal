#!/bin/bash
# Anchor on the script's own directory so 'go' works from anywhere.
BASE="$(cd "$(dirname "$0")" && pwd)"

if [ -f "$BASE/link_lab/original.txt" ]; then
    echo "[FAIL]: 'original.txt' still exists. Did you complete Step 5?"
    exit 1
fi

if [ ! -f "$BASE/link_lab/hard_link.txt" ]; then
    echo "[FAIL]: 'hard_link.txt' is missing (expected in /tmp/level2/link_lab)."
    exit 1
fi

CONTENT=$(cat "$BASE/link_lab/hard_link.txt" 2>/dev/null)
if [[ "$CONTENT" == *"Linux is awesome"* ]] && [[ "$CONTENT" == *"Learning links"* ]]; then
    echo "[PASS]: SUCCESS (Data intact)"
else
    echo "[FAIL]: 'hard_link.txt' does not contain the expected text."
    exit 1
fi

if [ ! -L "$BASE/link_lab/soft_link.txt" ]; then
    echo "[FAIL]: 'soft_link.txt' is missing or is not a symbolic link."
    exit 1
fi

if [ ! -e "$BASE/link_lab/soft_link.txt" ]; then
    echo "[PASS]: SUCCESS (Link is correctly broken/dangling)"
    exit 0
else
    echo "[FAIL]: 'soft_link.txt' is still pointing to a valid file."
    exit 1
fi
