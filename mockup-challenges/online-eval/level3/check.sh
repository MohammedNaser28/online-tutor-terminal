#!/bin/bash

USER_ANSWER="$1"
LOG_FILE="log"
SUCCESS=true

if grep -qi "not found" "$LOG_FILE" 2>/dev/null; then
    echo "[PASS] '$LOG_FILE' contains the correct 'not found' error message."
else
    echo "[FAIL] '$LOG_FILE' does not contain the expected error. Did you run 'path'?"
    SUCCESS=false
fi

ACTUAL_COUNT=$(echo "$PATH" | wc -l | xargs)

if [ "$USER_ANSWER" = "$ACTUAL_COUNT" ]; then
    echo "[PASS] Correct! You passed '$USER_ANSWER', which matches the exact number of lines in \$PATH."
else
    echo "[FAIL] Incorrect. You passed '$USER_ANSWER'. Check your 'echo \$PATH | wc -l' command again."
    SUCCESS=false
fi

if [ "$SUCCESS" = true ]; then
    exit 0
else
    exit 1
fi
