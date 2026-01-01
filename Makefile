.PHONY: help run build test clean lint format docker-build docker-run

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

run: ## Run the application
	@go run cmd/server/main.go

build: ## Build the application
	@echo "Building..."
	@go build -o bin/crawler cmd/server/main.go
	@echo "Build complete!"

test: ## Run tests
	@go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	@go tool cover -html=coverage.out

clean: ## Clean build artifacts
	@rm -rf bin/
	@rm -f coverage.out
	@echo "Clean complete!"

lint: ## Run linter
	@golangci-lint run

format: ## Format code
	@go fmt ./...
	@goimports -w .

deps: ## Download dependencies
	@go mod download
	@go mod tidy

docker-build: ## Build docker image
	@docker build -t crawl-news:latest .

docker-run: ## Run docker container
	@docker run -p 9000:9000 --env-file .env crawl-news:latest

dev: ## Run in development mode with hot reload (requires air)
	@air

install-tools: ## Install development tools
	@go install github.com/cosmtrek/air@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest
