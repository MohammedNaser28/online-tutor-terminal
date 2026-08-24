#!/bin/bash
# Rogue satellite daemon — heartbeats every second until killed.
while true; do
    echo "$(date '+%H:%M:%S') satellite heartbeat pid=$$" >> /tmp/level2/satellite.log
    sleep 1
done
