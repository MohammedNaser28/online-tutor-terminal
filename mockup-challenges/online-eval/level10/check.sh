#!/bin/bash

BASE="$(cd "$(dirname "$0")" && pwd)"
TARGET_SCRIPT="$BASE/sleeper.sh"

if [ -f "$TARGET_SCRIPT" ]; then
    echo "[PASS] '$TARGET_SCRIPT' exists."
else
    echo "[FAIL] '$TARGET_SCRIPT' does not exist."
    exit 1
fi

if cat "$TARGET_SCRIPT" | grep -q "sleep"; then
    echo "[PASS] Found a 'sleep' command inside '$TARGET_SCRIPT'."
else
    echo "[FAIL] No 'sleep' command found inside '$TARGET_SCRIPT'."
    exit 1
fi

# The brackets around [s]leep prevent the grep command itself from showing up in the process list.
if ps -ef | grep "[s]leep" > /dev/null; then
    echo "[FAIL] A sleep process was found still running. It was not killed successfully."
    ps -ef | grep "[s]leep"
    exit 1
else
    echo "[PASS] No active sleep process found. It was successfully killed."
    exit 0
fi
