#!/bin/bash
# Level 2 check: Did the student create a user named "studentuser"?

if id "studentuser" &>/dev/null; then
    echo "Level 2 passed!"
    exit 0
else
    echo "Level 2 failed: user 'studentuser' does not exist."
    exit 1
fi
