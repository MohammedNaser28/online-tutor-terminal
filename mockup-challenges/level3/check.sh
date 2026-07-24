#!/bin/bash
# Level 3 check: Was secret.txt copied and permissions set?

if [ -f "$HOME/secret.txt" ] && [ "$(stat -c "%a" "$HOME/secret.txt")" == "600" ]; then
    echo "Level 3 passed!"
    exit 0
else
    echo "Level 3 failed: Check file copy and permissions."
    exit 1
fi
