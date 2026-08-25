#!/bin/bash

if [ "$1" = "main" ]; then
    echo "[PASS] passed file name is the correct one"
    exit 0
else
    echo "[FAIL] not the right file"
    exit 1
fi
