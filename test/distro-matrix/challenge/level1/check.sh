#!/bin/bash
if [ -f /tmp/solved.txt ]; then
  echo "Correct!"
  exit 0
else
  echo "File not found"
  exit 1
fi
