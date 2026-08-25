#!/bin/bash

if [[ "$1" == "132" ]]; then
    echo "[PASS] correct key"
    exit 0
else
    echo "[FAIL] incorrect numericals or order"
    exit 1
fi
