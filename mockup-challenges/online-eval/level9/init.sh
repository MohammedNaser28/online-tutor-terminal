#!/bin/bash
# Relocate the hidden config tree to /temp inside the sandbox so the
# absolute path in the question is real. Runs host-side with write access
# to the sandbox rootfs; $PWD is the level directory.
CHROOT_DIR="$(dirname "$(dirname "$PWD")")"
if [ -d "$PWD/temp" ] && [ ! -d "$CHROOT_DIR/temp" ]; then
    cp -r "$PWD/temp" "$CHROOT_DIR/temp"
fi
