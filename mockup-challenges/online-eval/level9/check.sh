#!/bin/bash

LINK_NAME="app_config"
EXPECTED_TARGET="/temp/var/opt/app/hidden_config.cfg"

if [ ! -L "$LINK_NAME" ]; then
    if [ -e "$LINK_NAME" ]; then
        echo "[FAIL] '$LINK_NAME' exists but is NOT a symbolic link."
        echo "Did you use 'cp', or create a hard link by forgetting the '-s' flag?"
    else
        echo "[FAIL] '$LINK_NAME' does not exist in your current directory."
    fi
    exit 1
fi

RAW=$(readlink "$LINK_NAME" 2>/dev/null)
RESOLVED=$(readlink -f "$LINK_NAME" 2>/dev/null)

if [ "$RAW" = "$EXPECTED_TARGET" ] && [ -f "$LINK_NAME" ]; then
    echo "[PASS] '$LINK_NAME' correctly points at '$EXPECTED_TARGET'."
    exit 0
elif [ "$RESOLVED" = "$EXPECTED_TARGET" ] && [ -f "$LINK_NAME" ]; then
    echo "[PASS] '$LINK_NAME' correctly resolves to '$EXPECTED_TARGET'."
    exit 0
else
    echo "[FAIL] '$LINK_NAME' points at '${RESOLVED:-$RAW}', expected '$EXPECTED_TARGET'."
    exit 1
fi
