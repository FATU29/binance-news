#!/bin/bash

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Function to check if Docker is running
check_docker() {
    if ! docker info > /dev/null 2>&1; then
        print_error "Docker is not running. Please start Docker first."
        exit 1
    fi
    print_success "Docker is running"
}

# Function to check if docker-compose is available
check_docker_compose() {
    if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null 2>&1; then
        print_error "docker-compose is not installed"
        exit 1
    fi
    print_success "docker-compose is available"
}

# Function to create .env file if not exists
create_env_file() {
    if [ ! -f .env ]; then
        print_warning ".env file not found. Creating from .env.example..."
        cp .env.example .env
        print_success "Created .env file"
    else
        print_info ".env file already exists"
    fi
}

# Function to build images
build_images() {
    print_info "Building Docker images..."
    docker-compose build
    if [ $? -eq 0 ]; then
        print_success "Images built successfully"
    else
        print_error "Failed to build images"
        exit 1
    fi
}

# Function to start services
start_services() {
    MODE=${1:-production}
    
    if [ "$MODE" = "dev" ] || [ "$MODE" = "development" ]; then
        print_info "Starting services in DEVELOPMENT mode with hot reload..."
        docker-compose -f docker-compose.dev.yml up -d
    else
        print_info "Starting services in PRODUCTION mode..."
        docker-compose up -d
    fi
    
    if [ $? -eq 0 ]; then
        print_success "Services started successfully"
    else
        print_error "Failed to start services"
        exit 1
    fi
}

# Function to stop services
stop_services() {
    print_info "Stopping services..."
    docker-compose down
    docker-compose -f docker-compose.dev.yml down 2>/dev/null
    print_success "Services stopped"
}

# Function to show logs
show_logs() {
    SERVICE=${1:-crawl-service}
    print_info "Showing logs for $SERVICE..."
    docker-compose logs -f "$SERVICE"
}

# Function to check service health
check_health() {
    print_info "Checking service health..."
    
    echo ""
    print_info "Waiting for services to be healthy..."
    sleep 5
    
    # Check crawl service
    if curl -s http://localhost:9000/health > /dev/null; then
        print_success "Crawl Service: http://localhost:9000 ✓"
    else
        print_error "Crawl Service: http://localhost:9000 ✗"
    fi
    
    # Check PostgreSQL
    if docker exec crypto-postgres pg_isready -U postgres > /dev/null 2>&1; then
        print_success "PostgreSQL: localhost:5432 ✓"
    else
        print_warning "PostgreSQL: localhost:5432 ✗"
    fi
    
    # Check Redis
    if docker exec crypto-redis redis-cli ping > /dev/null 2>&1; then
        print_success "Redis: localhost:6379 ✓"
    else
        print_warning "Redis: localhost:6379 ✗"
    fi
    
    echo ""
    print_info "Service endpoints:"
    echo "  - Crawl API: http://localhost:9000"
    echo "  - Health Check: http://localhost:9000/health"
    echo "  - API Docs: http://localhost:9000/api/v1"
}

# Function to run crawler
run_crawler() {
    SOURCE=${1:-cointelegraph}
    print_info "Starting crawler for source: $SOURCE"
    
    curl -X POST http://localhost:9000/api/v1/crawler/start \
        -H "Content-Type: application/json" \
        -d "{\"source\": \"$SOURCE\"}"
    
    echo ""
    print_success "Crawler started. Check status at: http://localhost:9000/api/v1/crawler/status"
}

# Function to show usage
show_usage() {
    cat << EOF
${GREEN}Crypto News Crawler - Docker Management${NC}

${YELLOW}Usage:${NC}
  $0 [command] [options]

${YELLOW}Commands:${NC}
  ${BLUE}start [mode]${NC}      Start services (mode: prod|dev, default: prod)
  ${BLUE}stop${NC}              Stop all services
  ${BLUE}restart [mode]${NC}    Restart services
  ${BLUE}build${NC}             Build Docker images
  ${BLUE}logs [service]${NC}    Show logs (default: crawl-service)
  ${BLUE}health${NC}            Check service health
  ${BLUE}crawl [source]${NC}    Start crawler (default: cointelegraph)
  ${BLUE}clean${NC}             Stop services and remove volumes
  ${BLUE}ps${NC}                Show running containers

${YELLOW}Examples:${NC}
  $0 start              # Start in production mode
  $0 start dev          # Start in development mode with hot reload
  $0 logs               # Show crawl-service logs
  $0 logs postgres      # Show PostgreSQL logs
  $0 crawl coindesk     # Start crawler for CoinDesk
  $0 health             # Check all service health

EOF
}

# Main script
case "${1}" in
    start)
        check_docker
        check_docker_compose
        create_env_file
        start_services "${2}"
        sleep 3
        check_health
        ;;
    stop)
        stop_services
        ;;
    restart)
        stop_services
        sleep 2
        start_services "${2}"
        sleep 3
        check_health
        ;;
    build)
        check_docker
        check_docker_compose
        build_images
        ;;
    logs)
        show_logs "${2}"
        ;;
    health)
        check_health
        ;;
    crawl)
        run_crawler "${2}"
        ;;
    clean)
        print_warning "This will remove all containers and volumes!"
        read -p "Are you sure? (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            docker-compose down -v
            docker-compose -f docker-compose.dev.yml down -v 2>/dev/null
            print_success "Cleaned up all containers and volumes"
        fi
        ;;
    ps)
        docker-compose ps
        ;;
    *)
        show_usage
        exit 1
        ;;
esac
