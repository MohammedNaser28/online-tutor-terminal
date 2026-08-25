#!/bin/bash

SUCCESS=true

if [[ "$1" == "add" ]]; then
    echo "[PASS] changes added to staging area"
else
    echo "[FAIL] you didn't add your changes to the staging area"
    SUCCESS=false
fi

if [[ "$2" == "commit" ]]; then
    echo "[PASS] you are a committed individual!"
else
    echo "[FAIL] you forgot to commit your changes"
    SUCCESS=false
fi

if [[ "$3" == "push" ]]; then
    echo "[PASS] local was pushed to remote"
else
    echo "[FAIL] changes stayed local"
    SUCCESS=false
fi

output=$(cat ./project.java 2>/dev/null)

if [[ "$output" == 'System.out.println("lit is lit!")' ]]; then
    echo "[PASS] project code is correct"
else
    echo "[FAIL] project code is incorrect"
    SUCCESS=false
fi

if [ "$SUCCESS" = true ]; then
    echo "MISSION ACCOMPLISHED: lit is lit!"
    exit 0
else
    exit 1
fi
