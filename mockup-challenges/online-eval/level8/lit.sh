#!/bin/bash

if [[ "$1" == "add" ]]; then
	echo "[PASS] changes added to staging area"
else
	echo "[FAIL] you didn't add your changes to the staging area"
fi

if [[ "$2" == "commit" ]]; then
	echo "[PASS] you are a committed individual!"
else
	echo "[FAIL] you forgot to commit your changes"
fi

if [[ "$3" == "push" ]]; then
	echo "[PASS] local was pushed to remote"
else
	echo "[FAIL] changes stayed local"
fi

output=$(cat ./project.java)

if [[ "$output" == 'System.out.println("lit is lit!")' ]]; then
	echo "[PASS] project code is correct"
else
	echo "[FAIL] project code is incorrect"
fi
