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
SELECT format('CREATE ROLE diary_app LOGIN PASSWORD %L', :'app_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'diary_app') \gexec
SELECT format('CREATE ROLE diary_litellm LOGIN PASSWORD %L', :'gateway_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'diary_litellm') \gexec
SELECT 'CREATE DATABASE diary_litellm OWNER diary_litellm'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'diary_litellm') \gexec
GRANT CONNECT ON DATABASE diary_listener TO diary_app;
GRANT USAGE ON SCHEMA public TO diary_app;
ALTER DEFAULT PRIVILEGES FOR ROLE diary_migrator IN SCHEMA public
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO diary_app;
ALTER DEFAULT PRIVILEGES FOR ROLE diary_migrator IN SCHEMA public
    GRANT USAGE, SELECT ON SEQUENCES TO diary_app;
SQL
