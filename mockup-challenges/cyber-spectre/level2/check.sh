#!/bin/bash
# Ensure sentry.sh was started at least once
if [ ! -f "/tmp/.sentry_started" ]; then
  touch /tmp/.sentry_started
  chmod +x ./sentry.sh
  ./sentry.sh >/dev/null 2>&1 &
  exit 1
fi

# Check if sentry process is still running
if pgrep -f "sentry.sh" > /dev/null 2>&1; then
  echo "Sentry process is still active in background!"
  exit 1
fi

# Check if execute permission was revoked
if [ -x "./sentry.sh" ]; then
  echo "sentry.sh still has execute (+x) permissions!"
  exit 1
fi

exit 0
