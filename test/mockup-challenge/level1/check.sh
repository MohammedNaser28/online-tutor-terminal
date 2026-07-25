#!/bin/bash
read -p "Your answer: " ans
if [ "$ans" = "4" ]; then
  echo "Correct!"
  exit 0
else
  echo "Wrong, try again."
  exit 1
fi
