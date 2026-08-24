#!/bin/bash
# Level 1: verify the submitted uplink code.
# Accepts the code as $1 (go QO-...) or reads it from stdin.
ans="${1}"
if [ -z "$ans" ]; then
    read -r ans
fi
clean=$(echo "$ans" | tr -d '\r\n ')
expected=$(cat /tmp/level1/.hidden/key.txt 2>/dev/null | tr -d '\r\n ')
if [ "$clean" = "$expected" ] && [ -n "$expected" ]; then
    echo "Uplink code accepted. Channel to base re-established!"
    exit 0
else
    echo "Invalid uplink code. Keep searching, agent."
    exit 1
fi
