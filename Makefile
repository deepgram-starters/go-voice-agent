# Go Voice Agent Makefile
# Framework-agnostic commands for managing the project and git submodules

PNPM := corepack pnpm

.PHONY: help check check-prereqs init install install-frontend build start start-backend start-frontend test update clean status eject-frontend

# Default target: show help
help:
	@echo "Go Voice Agent - Available Commands"
	@echo "===================================="
	@echo ""
	@echo "Setup:"
	@echo "  make check-prereqs     Check required tools are installed"
	@echo "  make init              Initialize submodules and install all dependencies"
	@echo "  make install           Install backend dependencies only"
	@echo "  make install-frontend  Install frontend dependencies only"
	@echo ""
	@echo "Development:"
	@echo "  make build             Build frontend for production"
	@echo "  make start             Start both backend and frontend servers in parallel"
	@echo "  make start-backend     Start backend API server only (port 8081)"
	@echo "  make start-frontend    Start frontend dev server only (port 8080)"
	@echo ""
	@echo "Maintenance:"
	@echo "  make update            Update submodules to latest commits"
	@echo "  make clean             Remove build artifacts"
	@echo "  make status            Show git and submodule status"
	@echo ""

# Check required prerequisites
check-prereqs:
	@command -v git >/dev/null 2>&1 || { echo "git is required but not installed. Visit https://git-scm.com"; exit 1; }
	@command -v go >/dev/null 2>&1 || { echo "go is required but not installed. Visit https://go.dev"; exit 1; }
	@echo "All prerequisites installed"

# Initialize project: clone submodules and install dependencies
init: check-prereqs
	@echo "==> Initializing submodules..."
	git submodule update --init --recursive
	@echo ""
	@echo "==> Installing backend dependencies..."
	go mod tidy
	@echo ""
	@echo "==> Installing frontend dependencies..."
	cd frontend && $(PNPM) install
	@echo ""
	@echo "Project initialized successfully!"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Copy sample.env to .env and add your DEEPGRAM_API_KEY"
	@echo "  2. Run 'make start' to start development servers"
	@echo ""

# Install backend dependencies
install:
	@echo "==> Installing backend dependencies..."
	go mod tidy

# Install frontend dependencies (requires submodule to be initialized)
install-frontend:
	@echo "==> Installing frontend dependencies..."
	@if [ ! -d "frontend" ] || [ -z "$$(ls -A frontend)" ]; then \
		echo "Error: Frontend submodule not initialized. Run 'make init' first."; \
		exit 1; \
	fi
	cd frontend && $(PNPM) install

# Build frontend for production
build:
	@if [ ! -d "frontend" ] || [ -z "$$(ls -A frontend)" ]; then \
		echo "Error: Frontend submodule not initialized. Run 'make init' first."; \
		exit 1; \
	fi
	@echo "==> Building frontend..."
	cd frontend && $(PNPM) run build

# Start both servers in parallel
start:
	@$(MAKE) start-backend & $(MAKE) start-frontend & wait

# Start backend API server only
start-backend:
	@if [ ! -f ".env" ]; then \
		echo "Error: .env file not found. Copy sample.env to .env and add your DEEPGRAM_API_KEY"; \
		exit 1; \
	fi
	@echo "==> Starting backend on http://localhost:8081"
	set -a && . ./.env && set +a && go run main.go

# Start frontend dev server only
start-frontend:
	@if [ ! -d "frontend" ] || [ -z "$$(ls -A frontend)" ]; then \
		echo "Error: Frontend submodule not initialized. Run 'make init' first."; \
		exit 1; \
	fi
	@echo "==> Starting frontend on http://localhost:8080"
	cd frontend && $(PNPM) run dev -- --port 8080 --no-open

# Alias for check-prereqs (matches deepgram.toml [check] section)
check: check-prereqs

# Run contract conformance tests
test:
	@if [ ! -f ".env" ]; then \
		echo "Error: .env file not found. Copy sample.env to .env and add your DEEPGRAM_API_KEY"; \
		exit 1; \
	fi
	@if [ ! -d "contracts" ] || [ -z "$$(ls -A contracts)" ]; then \
		echo "Error: Contracts submodule not initialized. Run 'make init' first."; \
		exit 1; \
	fi
	@echo "==> Running contract conformance tests..."
	@bash contracts/tests/run-voice-agent-app.sh

# Update submodules to latest commits
update:
	@echo "==> Updating submodules..."
	git submodule update --remote --merge
	@echo "Submodules updated"

# Clean all dependencies and build artifacts
clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf frontend/node_modules
	rm -rf frontend/dist
	@echo "Cleaned successfully"

# Show git and submodule status
status:
	@echo "==> Repository Status"
	@echo "====================="
	@echo ""
	@echo "Main Repository:"
	git status --short
	@echo ""
	@echo "Submodule Status:"
	git submodule status
	@echo ""
	@echo "Submodule Branches:"
	@cd frontend && echo "frontend: $$(git branch --show-current) ($$(git rev-parse --short HEAD))"

eject-frontend:
	@echo ""
	@echo "This will:"
	@echo "   1. Copy frontend submodule files into a regular 'frontend/' directory"
	@echo "   2. Remove the frontend git submodule configuration"
	@echo "   3. Remove the contracts git submodule"
	@echo "   4. Remove .gitmodules file"
	@echo ""
	@echo "   After ejecting, frontend changes can be committed directly"
	@echo "   with your backend changes. This cannot be undone."
	@echo ""
	@read -p "   Continue? [Y/n] " confirm; \
	if [ "$$confirm" != "Y" ] && [ "$$confirm" != "y" ] && [ -n "$$confirm" ]; then \
		echo "   Cancelled."; \
		exit 1; \
	fi
	@echo ""
	@echo "==> Ejecting frontend submodule..."
	@FRONTEND_TMP=$$(mktemp -d); \
	cp -r frontend/. "$$FRONTEND_TMP/"; \
	git submodule deinit -f frontend; \
	git rm -f frontend; \
	rm -rf .git/modules/frontend; \
	mkdir -p frontend; \
	cp -r "$$FRONTEND_TMP/." frontend/; \
	rm -rf "$$FRONTEND_TMP"; \
	rm -rf frontend/.git; \
	echo "   Frontend ejected to regular directory"
	@echo "==> Removing contracts submodule..."
	@if git config --file .gitmodules submodule.contracts.url > /dev/null 2>&1; then \
		git submodule deinit -f contracts; \
		git rm -f contracts; \
		rm -rf .git/modules/contracts; \
		echo "   Contracts submodule removed"; \
	else \
		echo "   No contracts submodule found"; \
	fi
	@if [ -f .gitmodules ] && [ ! -s .gitmodules ]; then \
		git rm -f .gitmodules; \
		echo "   Empty .gitmodules removed"; \
	fi
	@echo ""
	@echo "Eject complete! Frontend files are now regular tracked files."
	@echo "   Run 'git add . && git commit' to save the changes."
