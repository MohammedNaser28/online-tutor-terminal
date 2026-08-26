#!/bin/bash

success=true

if grep -q '^penguin:' /etc/passwd; then
    echo "[PASS] User 'penguin' exists."
else
    echo "[FAIL] User 'penguin' does not exist."
    success=false
fi

if grep -q '^pengroup:' /etc/group; then
    echo "[PASS] Group 'pengroup' exists."
else
    echo "[FAIL] Group 'pengroup' does not exist."
    success=false
fi

if id -gn penguin 2>/dev/null | grep -q '^pengroup$'; then
    echo "[PASS] Primary group for 'penguin' is correctly set to 'pengroup'."
else
    current_group=$(id -gn penguin 2>/dev/null)
    echo "[FAIL] Primary group is '$current_group', expected 'pengroup'."
    success=false
fi

if ! grep -q '^penguin:' /etc/group; then
    echo "[PASS] Old 'penguin' group does not exist (successfully removed)."
else
    echo "[FAIL] Old 'penguin' group still exists in /etc/group."
    success=false
fi

if [ "$success" = "true" ]; then
    echo "SUCCESS"
    exit 0
else
    echo "FAILED"
    exit 1
fi
