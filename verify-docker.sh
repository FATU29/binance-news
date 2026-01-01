#!/bin/bash

# Quick verification script for Docker setup
# Run this after starting Docker

set -e

echo "🔍 Verifying Docker Setup..."
echo ""

# Check Docker
echo "1️⃣ Checking Docker..."
if docker info > /dev/null 2>&1; then
    echo "✅ Docker is running"
else
    echo "❌ Docker is not running. Please start Docker Desktop first."
    exit 1
fi

# Check docker-compose
echo ""
echo "2️⃣ Checking docker-compose..."
if command -v docker-compose &> /dev/null || docker compose version &> /dev/null 2>&1; then
    echo "✅ docker-compose is available"
else
    echo "❌ docker-compose is not installed"
    exit 1
fi

# Build production image
echo ""
echo "3️⃣ Building production image..."
docker build -t crypto-crawl:latest . --no-cache

# Check image size
echo ""
echo "4️⃣ Checking image size..."
docker images crypto-crawl:latest

# Verify .env file
echo ""
echo "5️⃣ Checking .env file..."
if [ -f .env ]; then
    echo "✅ .env file exists"
else
    echo "⚠️  Creating .env from .env.example..."
    cp .env.example .env
    echo "✅ Created .env file"
fi

# Summary
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Docker setup verified successfully!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "📋 Quick Commands:"
echo "  ./docker-start.sh start       # Start production"
echo "  ./docker-start.sh start dev   # Start development"
echo "  ./docker-start.sh health      # Check health"
echo "  ./docker-start.sh logs        # View logs"
echo ""
echo "🚀 Ready to start services!"
