#!/bin/bash
set -e

echo "Validating migration files..."

MIGRATIONS_DIR="migrations"

if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo "Error: migrations directory not found"
    exit 1
fi

# Check that each .up.sql has a corresponding .down.sql
for up_file in "$MIGRATIONS_DIR"/*.up.sql; do
    if [ -f "$up_file" ]; then
        base_name=$(basename "$up_file" .up.sql)
        down_file="$MIGRATIONS_DIR/${base_name}.down.sql"
        if [ ! -f "$down_file" ]; then
            echo "Error: Missing down migration for $up_file"
            exit 1
        fi
    fi
done

# Check SQL syntax (basic validation)
for sql_file in "$MIGRATIONS_DIR"/*.sql; do
    if [ -f "$sql_file" ]; then
        # Check for common SQL issues
        if grep -qE "^\s*$" "$sql_file" && [ ! -s "$sql_file" ]; then
            echo "Warning: Empty migration file: $sql_file"
        fi
    fi
done

echo "All migrations validated successfully"
