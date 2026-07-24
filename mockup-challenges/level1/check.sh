#!/bin/bash
# Level 1 check: Did the student create a folder called "testdir"?

if [ -d "$PWD/testdir" ]; then
    echo "Level 1 passed!"
    exit 0
else
    echo "Level 1 failed: 'testdir' does not exist."
    exit 1
fi
