#!/bin/bash

# Script to run database migrations
# Usage: ./scripts/run-migrations.sh

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}==================================${NC}"
echo -e "${GREEN}Database Migration Runner${NC}"
echo -e "${GREEN}==================================${NC}"

# Load environment variables
if [ -f .env ]; then
    source .env
fi

# Default values
DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-postgres}
DB_PASSWORD=${DB_PASSWORD:-crypto_password}
DB_NAME=${DB_NAME:-crypto_news}

echo -e "\n${YELLOW}Configuration:${NC}"
echo "  Host: $DB_HOST"
echo "  Port: $DB_PORT"
echo "  Database: $DB_NAME"
echo "  User: $DB_USER"

# Check if PostgreSQL is accessible
echo -e "\n${YELLOW}Checking database connection...${NC}"
if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c '\q' 2>/dev/null; then
    echo -e "${GREEN}✓ Database connection successful${NC}"
else
    echo -e "${RED}✗ Cannot connect to database${NC}"
    echo -e "${YELLOW}Make sure PostgreSQL is running and credentials are correct${NC}"
    exit 1
fi

# Run migrations
echo -e "\n${YELLOW}Running migrations...${NC}"

MIGRATION_DIR="./migrations"

if [ ! -d "$MIGRATION_DIR" ]; then
    echo -e "${RED}✗ Migrations directory not found: $MIGRATION_DIR${NC}"
    exit 1
fi

# Get list of migration files
MIGRATIONS=$(ls $MIGRATION_DIR/*.sql 2>/dev/null | sort)

if [ -z "$MIGRATIONS" ]; then
    echo -e "${YELLOW}No migration files found${NC}"
    exit 0
fi

# Run each migration
for migration in $MIGRATIONS; then
    filename=$(basename $migration)
    echo -e "\n${YELLOW}Applying migration: $filename${NC}"
    
    if PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f $migration; then
        echo -e "${GREEN}✓ $filename applied successfully${NC}"
    else
        echo -e "${RED}✗ Failed to apply $filename${NC}"
        exit 1
    fi
done

echo -e "\n${GREEN}==================================${NC}"
echo -e "${GREEN}All migrations completed!${NC}"
echo -e "${GREEN}==================================${NC}"

# Display table information
echo -e "\n${YELLOW}Database tables:${NC}"
PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -c "\dt"

exit 0

