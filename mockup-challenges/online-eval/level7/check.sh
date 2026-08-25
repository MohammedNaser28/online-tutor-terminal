#!/bin/bash

if [[ "$1" == "5" && "$2" == "10" ]]; then
    echo "[PASS], person numbers correct"
    exit 0
else
    echo "[FAIL], not a file exactly 55 bytes or not a person whose date is greater than 2000."
    echo "Usage: ./check.sh <first-person> <second-person>  (ascending order)"
    exit 1
fi
