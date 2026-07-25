#!/bin/bash
read -p "Your answer: " ans
if [ "$ans" = "paris" ] || [ "$ans" = "Paris" ]; then
  echo "Correct!"
  exit 0
else
  echo "Wrong, try again."
  exit 1
fi
