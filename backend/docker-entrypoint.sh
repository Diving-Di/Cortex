#!/bin/sh
set -eu

mkdir -p /app/data/attachments /app/data/media/cortex
chown -R cortex:cortex /app/data

su-exec cortex:cortex /app/migrate -steps 0 up
exec su-exec cortex:cortex /app/server
