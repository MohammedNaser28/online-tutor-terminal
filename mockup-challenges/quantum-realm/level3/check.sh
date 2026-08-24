#!/bin/bash
# Level 3: master.key must be mode 600.
KEY=/tmp/level3/master.key

mode=$(stat -c "%a" "$KEY" 2>/dev/null)
if [ "$mode" = "600" ]; then
    echo "Master key secured (mode 600). Extraction window open — you're done, agent! 🎉"
    exit 0
else
    echo "Key permissions are '$mode' — need exactly 600 (owner rw, nothing else)."
    exit 1
fi
