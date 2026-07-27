#!/bin/sh
set -eu

mkdir -p /app/data/attachments /app/data/media/diary
chown -R diary:diary /app/data

su-exec diary:diary /app/migrate -steps 0 up
exec su-exec diary:diary /app/server
