#!/bin/bash
# Relocate the hidden config tree to /temp inside the sandbox so the
# absolute path in the question is real. Runs host-side with write access
# to the sandbox rootfs; $PWD is this level's directory.
D="$PWD"
while [ "$D" != "/" ] && [ "$(basename "$D")" != "rootfs" ]; do
    D="$(dirname "$D")"
done
if [ -d "$PWD/temp" ] && [ "$(basename "$D")" = "rootfs" ] && [ ! -d "$D/temp" ]; then
    cp -r "$PWD/temp" "$D/temp"
fi
