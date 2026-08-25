#!/bin/bash

if [ -z "$1" ]; then
    echo "[FAIL] No file provided. You must pass the file with the most punctuation as an argument."
    exit 1
fi

if [ "$1" == "Random.txt" ]; then
    echo "[PASS] Correct! You identified '$1' as the file with the most punctuation."
else
    echo "[FAIL] Incorrect file. You passed '$1', which is not the file with the maximum punctuation."
fi


TARGET_FILES=("Random.txt" "Debian.txt" "Fedora.txt")
ALL_EXTRACTED=true

for file in "${TARGET_FILES[@]}"; do
    if [ -f "$file" ]; then
        count=$(grep -o '[[:punct:]]' "$file" | wc -l)
        echo " - [OK] '$file' exists. (Punctuation count: $count)"
    else
        echo " - [MISSING] '$file' does not exist. Did you extract it properly?"
        ALL_EXTRACTED=false
    fi
done

if [ "$ALL_EXTRACTED" = true ] && [ "$1" == "Random.txt" ]; then
    echo "SUCCESS: Mission accomplished! All files extracted and the correct file was submitted."
    exit 0
else
    echo "MISSION FAILED: Please check your extraction steps and your math."
    exit 1
fi
