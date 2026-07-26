#!/bin/sh
set -eu

mkdir -p /app/data/attachments /app/data/media/diary
chown -R diary:diary /app/data

exec su-exec diary:diary /app/server
