#!/bin/sh
set -eu

mkdir -p /app/data/attachments /app/data/media/cortex
chown -R cortex:cortex /app/data

su-exec cortex:cortex /app/migrate -steps 0 up
case "${CORTEX_PROCESS:-server}" in
  server|outbox-relay|file-gc-consumer|projection-consumer|knowledge-consumer) exec su-exec cortex:cortex "/app/${CORTEX_PROCESS:-server}" ;;
  *) echo "unsupported CORTEX_PROCESS" >&2; exit 64 ;;
esac
