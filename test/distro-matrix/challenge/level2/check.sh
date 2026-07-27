#!/bin/bash
if grep -q "correct_flag" /tmp/flag.txt 2>/dev/null; then
  echo "Correct!"
  exit 0
else
  echo "Flag not found"
  exit 1
fi
