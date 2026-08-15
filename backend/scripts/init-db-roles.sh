#!/bin/sh
set -eu

: "${POSTGRES_APP_PASSWORD:?POSTGRES_APP_PASSWORD must be set}"
: "${LITELLM_DB_PASSWORD:?LITELLM_DB_PASSWORD must be set}"
APP_PASSWORD="$POSTGRES_APP_PASSWORD"
GATEWAY_PASSWORD="$LITELLM_DB_PASSWORD"

psql --set=ON_ERROR_STOP=1 \
    --username "$POSTGRES_USER" \
    --dbname "$POSTGRES_DB" \
    --set=app_password="$APP_PASSWORD" \
    --set=gateway_password="$GATEWAY_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE cortex_app LOGIN PASSWORD %L', :'app_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'cortex_app') \gexec
SELECT format('CREATE ROLE cortex_litellm LOGIN PASSWORD %L', :'gateway_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'cortex_litellm') \gexec
SELECT 'CREATE DATABASE cortex_litellm OWNER cortex_litellm'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'cortex_litellm') \gexec
GRANT CONNECT ON DATABASE cortex TO cortex_app;
GRANT USAGE ON SCHEMA public TO cortex_app;
ALTER DEFAULT PRIVILEGES FOR ROLE cortex_migrator IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO cortex_app;
ALTER DEFAULT PRIVILEGES FOR ROLE cortex_migrator IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO cortex_app;
SQL
