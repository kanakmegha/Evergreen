#!/bin/sh
set -eu

# Prefer DIRECT_URL (unpooled direct connection for Neon) if set, otherwise fallback to DATABASE_URL.
TARGET_URL="${DIRECT_URL:-${DATABASE_URL:-}}"
if [ -z "$TARGET_URL" ]; then
  echo "Error: Set DIRECT_URL or DATABASE_URL to target PostgreSQL database" >&2
  exit 1
fi

psql "$TARGET_URL" -v ON_ERROR_STOP=1 -c "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"
for file in db/migrations/*.sql; do
  version=$(basename "$file" .sql)
  applied=$(psql "$TARGET_URL" -v ON_ERROR_STOP=1 -Atc "SELECT 1 FROM schema_migrations WHERE version='$version'")
  if [ "$applied" = "1" ]; then
    continue
  fi
  psql "$TARGET_URL" -v ON_ERROR_STOP=1 -1 -f "$file" -c "INSERT INTO schema_migrations(version) VALUES ('$version')"
  printf 'Applied %s\n' "$version"
done
