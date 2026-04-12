#!/bin/bash
set -e

echo "EduExchange: Waiting for database..."

# Wait for PostgreSQL to be ready
until PGPASSWORD=eduexchange psql -h db -U eduexchange -d eduexchange -c '\q' 2>/dev/null; do
  echo "  Database not ready, retrying in 2s..."
  sleep 2
done

echo "EduExchange: Database is ready."

# Run migrations
echo "EduExchange: Running migrations..."
./eduexchange migrate

# Seed if database is empty
USER_COUNT=$(PGPASSWORD=eduexchange psql -h db -U eduexchange -d eduexchange -tAc "SELECT COUNT(*) FROM users" 2>/dev/null || echo "0")
if [ "$USER_COUNT" = "0" ]; then
  echo "EduExchange: Seeding database..."
  ./eduexchange seed
fi

echo "EduExchange: Starting server on port 8080..."
exec ./eduexchange serve
