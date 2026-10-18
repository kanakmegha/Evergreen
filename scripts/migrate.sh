#!/bin/sh
set -eu
: "${DATABASE_URL:?Set DATABASE_URL to the target PostgreSQL database}"
psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -c "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())"
for file in db/migrations/*.sql; do
  version=$(basename "$file" .sql)
  applied=$(psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -Atc "SELECT 1 FROM schema_migrations WHERE version='$version'")
  if [ "$applied" = "1" ]; then
    continue
  fi
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -1 -f "$file" -c "INSERT INTO schema_migrations(version) VALUES ('$version')"
  printf 'Applied %s\n' "$version"
done
