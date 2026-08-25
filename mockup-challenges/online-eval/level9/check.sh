#!/bin/bash

LINK_NAME="app_config"
EXPECTED_TARGET="/temp/var/opt/app/hidden_config.cfg"

LINK_PATH="$HOME/app_config"

if [ ! -L "$LINK_PATH" ]; then
    if [ -e "$LINK_PATH" ]; then
        echo "[FAIL] '$LINK_PATH' exists but is NOT a symbolic link."
        echo "Did you use 'cp', or create a hard link by forgetting the '-s' flag?"
    else
        echo "[FAIL] '$LINK_PATH' does not exist in your home directory ($HOME)."
    fi
    exit 1
fi

RAW=$(readlink "$LINK_PATH" 2>/dev/null)
RESOLVED=$(readlink -f "$LINK_PATH" 2>/dev/null)

if [ "$RAW" = "$EXPECTED_TARGET" ] && [ -f "$LINK_PATH" ]; then
    echo "[PASS] '$LINK_PATH' correctly points at '$EXPECTED_TARGET'."
    exit 0
elif [ "$RESOLVED" = "$EXPECTED_TARGET" ] && [ -f "$LINK_PATH" ]; then
    echo "[PASS] '$LINK_PATH' correctly resolves to '$EXPECTED_TARGET'."
    exit 0
else
    echo "[FAIL] '$LINK_PATH' points at '${RESOLVED:-$RAW}', expected '$EXPECTED_TARGET'."
    exit 1
fi
