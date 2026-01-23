#!/bin/bash

# Script to clear database and start fresh crawl with AI-only mode

set -e

echo "🚀 Starting fresh crawl with AI-only parsing..."

# Step 1: Clear database
echo ""
echo "Step 1: Clearing database..."
./scripts/clear-database.sh

# Step 2: Wait for services to be ready
echo ""
echo "Step 2: Waiting for services to be ready..."
sleep 5

# Step 3: Start crawl with AI-only mode
echo ""
echo "Step 3: Starting crawl with AI-only mode..."
echo "⚠️  Make sure AI_SERVICE_URL and AI_FORCE_PARSING=true are set in environment"

# Example: Start crawl for a source
# You can modify this to crawl specific sources
SOURCES=("cointelegraph" "coindesk" "cryptonews")

for source in "${SOURCES[@]}"; do
    echo ""
    echo "📰 Crawling source: $source"
    curl -X POST http://localhost:9000/api/v1/crawler/start \
        -H "Content-Type: application/json" \
        -d "{\"source\": \"$source\"}" || echo "Failed to start crawl for $source"
done

echo ""
echo "✅ Crawl started! Check logs for progress."
