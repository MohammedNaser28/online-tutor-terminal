#!/bin/bash
# advanced check — verifies a process is running
pgrep -x "sleep" > /dev/null 2>&1
