#!/bin/bash
#
# MIDAS Database Setup
# Creates the MIDAS database from scratch
#
# Usage:
#   ./setup-db.sh                    # Interactive: prompts for database name
#   ./setup-db.sh midas              # Direct: uses 'midas' as database name
#   ./setup-db.sh -h                 # Show help

set -e

DB_NAME="${1:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCHEMA_FILE="${SCRIPT_DIR}/schema.sql"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

show_help() {
    cat << EOF
MIDAS Database Setup

Creates a fresh PostgreSQL database with the complete MIDAS schema.

Usage:
    $0 [DATABASE_NAME]
    $0 -h | --help

Arguments:
    DATABASE_NAME    Name for the database (default: prompts interactively)

Options:
    -h, --help      Show this help message

Examples:
    $0              # Interactive mode
    $0 midas        # Create database named 'midas'

Environment Variables:
    PGHOST          PostgreSQL host (default: localhost)
    PGPORT          PostgreSQL port (default: 5432)
    PGUSER          PostgreSQL user (default: postgres)
    PGPASSWORD      PostgreSQL password

The script will:
    1. Check if the database exists
    2. Create the database if needed
    3. Apply the complete MIDAS schema
    4. Verify the installation

EOF
}

# Parse arguments
if [[ "$1" == "-h" || "$1" == "--help" ]]; then
    show_help
    exit 0
fi

# Get database name
if [[ -z "$DB_NAME" ]]; then
    echo -e "${YELLOW}Enter database name (default: midas):${NC} "
    read -r DB_NAME
    DB_NAME="${DB_NAME:-midas}"
fi

# Validate database name
if [[ ! "$DB_NAME" =~ ^[a-zA-Z][a-zA-Z0-9_]*$ ]]; then
    echo -e "${RED}Error: Invalid database name '$DB_NAME'${NC}"
    echo "Database name must start with a letter and contain only letters, numbers, and underscores"
    exit 1
fi

# Check if schema file exists
if [[ ! -f "$SCHEMA_FILE" ]]; then
    echo -e "${RED}Error: Schema file not found: $SCHEMA_FILE${NC}"
    exit 1
fi

# Database connection defaults
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-postgres}"

echo -e "${GREEN}MIDAS Database Setup${NC}"
echo "===================="
echo "Database: $DB_NAME"
echo "Host:     $PGHOST:$PGPORT"
echo "User:     $PGUSER"
echo ""

# Check if database exists
echo -n "Checking if database exists... "
if psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -lqt | cut -d \| -f 1 | grep -qw "$DB_NAME"; then
    echo -e "${YELLOW}EXISTS${NC}"
    echo ""
    echo -e "${YELLOW}Warning: Database '$DB_NAME' already exists.${NC}"
    echo -n "Drop and recreate? (yes/no): "
    read -r CONFIRM
    if [[ "$CONFIRM" != "yes" ]]; then
        echo "Aborted."
        exit 0
    fi
    echo -n "Dropping database... "
    psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -c "DROP DATABASE $DB_NAME;" > /dev/null
    echo -e "${GREEN}OK${NC}"
fi

# Create database
echo -n "Creating database... "
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -c "CREATE DATABASE $DB_NAME;" > /dev/null
echo -e "${GREEN}OK${NC}"

# Apply schema
echo -n "Applying schema... "
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$DB_NAME" -f "$SCHEMA_FILE" > /dev/null
echo -e "${GREEN}OK${NC}"

# Verify installation
echo -n "Verifying tables... "
TABLE_COUNT=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$DB_NAME" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';")
EXPECTED_TABLES=6

if [[ "$TABLE_COUNT" -eq "$EXPECTED_TABLES" ]]; then
    echo -e "${GREEN}OK${NC} ($TABLE_COUNT tables created)"
else
    echo -e "${RED}FAILED${NC}"
    echo "Expected $EXPECTED_TABLES tables, found $TABLE_COUNT"
    exit 1
fi

# Show table list
echo ""
echo "Created tables:"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$DB_NAME" -c "\dt" | grep -E "^\s+(public\.|table)" || true

echo ""
echo -e "${GREEN}✓ Database setup complete!${NC}"
echo ""
echo "Connection string:"
echo "  postgresql://$PGUSER@$PGHOST:$PGPORT/$DB_NAME"
echo ""
echo "To connect:"
echo "  psql -h $PGHOST -p $PGPORT -U $PGUSER -d $DB_NAME"
