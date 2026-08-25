#!/bin/bash

if [ -f "original.txt" ]; then
    echo "[FAIL]: 'original.txt' still exists. Did you complete Step 5?"
    exit 1
fi

if [ ! -f "hard_link.txt" ]; then
    echo "[FAIL]: 'hard_link.txt' is missing."
    exit 1
fi

CONTENT=$(cat hard_link.txt 2>/dev/null)
if [[ "$CONTENT" == *"Linux is awesome"* ]] && [[ "$CONTENT" == *"Learning links"* ]]; then
    echo "[PASS]: SUCCESS (Data intact)"
else
    echo "[FAIL]: 'hard_link.txt' does not contain the expected text."
    exit 1
fi

if [ ! -L "soft_link.txt" ]; then
    echo "[FAIL]: 'soft_link.txt' is missing or is not a symbolic link."
    exit 1
fi

if [ ! -e "soft_link.txt" ]; then
    echo "[PASS]: SUCCESS (Link is correctly broken/dangling)"
    exit 0
else
    echo "[FAIL]: 'soft_link.txt' is still pointing to a valid file."
    exit 1
fi
