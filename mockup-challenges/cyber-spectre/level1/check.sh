#!/bin/bash
read -r ans
clean_ans=$(echo "$ans" | tr -d '\r\n ')
if [ "$clean_ans" = "CYBER-7731-SPECTRE" ]; then
  exit 0
else
  exit 1
fi
