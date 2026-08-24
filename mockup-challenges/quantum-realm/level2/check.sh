#!/bin/bash
# Level 2: satellite daemon must have heartbeat AND be dead.
LOG=/tmp/level2/satellite.log

if [ ! -s "$LOG" ]; then
    echo "satellite.log is empty — you never launched the daemon ('./satellite.sh &')."
    exit 1
fi

if pgrep -f "satellite.sh" > /dev/null 2>&1; then
    echo "The satellite is STILL RUNNING. Shut it down with kill."
    exit 1
fi

echo "Satellite daemon confirmed: heartbeats received and process terminated."
exit 0
