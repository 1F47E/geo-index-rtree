# Go Geo-Index Makefile
# High-performance R-Tree geographical indexing demo

.PHONY: all build clean test benchmark help install-deps

# Variables
BINARY_NAME=go-geo-index
INDEX_FILE=geo_index.gob
GO=go
GOFLAGS=-ldflags="-s -w"
GOBUILD=$(GO) build $(GOFLAGS)
GOTEST=$(GO) test
GOGET=$(GO) get

# Default number of points and queries
POINTS ?= 1000000
QUERIES ?= 1000
WORKERS ?= $(shell nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)
RADIUS ?= 50
NEIGHBORS ?= 10

all: build

help:
	@echo "Go Geo-Index - High-performance R-Tree implementation"
	@echo ""
	@echo "Available commands:"
	@echo "  make build          - Build the binary"
	@echo "  make install-deps   - Install Go dependencies"
	@echo "  make clean          - Remove binary and index files"
	@echo "  make test           - Run tests"
	@echo ""
	@echo "Demo commands:"
	@echo "  make demo           - Run complete demo (load + all benchmarks)"
	@echo "  make load-1m        - Load 1 million random points"
	@echo "  make load-10m       - Load 10 million random points"
	@echo "  make load-100m      - Load 100 million random points"
	@echo "  make load           - Load custom number of points (POINTS=1000000)"
	@echo ""
	@echo "Benchmark commands:"
	@echo "  make benchmark      - Run bounding box queries benchmark"
	@echo "  make bench-radius   - Run radius search benchmark"
	@echo "  make bench-nearest  - Run nearest neighbor benchmark"
	@echo "  make bench-all      - Run all benchmarks"
	@echo ""
	@echo "Environment variables:"
	@echo "  POINTS    - Number of points to generate (default: 1000000)"
	@echo "  QUERIES   - Number of queries to run (default: 1000)"
	@echo "  WORKERS   - Number of worker goroutines (default: CPU count)"
	@echo "  RADIUS    - Search radius in km (default: 50)"
	@echo "  NEIGHBORS - Number of nearest neighbors (default: 10)"
	@echo ""
	@echo "Examples:"
	@echo "  make load POINTS=5000000              - Load 5 million points"
	@echo "  make benchmark QUERIES=10000          - Run 10k queries"
	@echo "  make bench-radius RADIUS=100          - Search within 100km radius"
	@echo "  make bench-all QUERIES=5000 WORKERS=8 - Run all benchmarks with 8 workers"

build:
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o $(BINARY_NAME) ./cmd/main.go
	@echo "Build complete!"

install-deps:
	@echo "Installing dependencies..."
	$(GO) mod download
	$(GO) mod tidy
	@echo "Dependencies installed!"

clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -f $(INDEX_FILE)
	@rm -f *.gob
	@echo "Clean complete!"

test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Demo commands
demo: build
	@echo "Running interactive demo with colorful output..."
	@$(GO) run ./cmd/demo/demo.go

load: build
	@echo "Loading $(POINTS) points using $(WORKERS) workers..."
	./$(BINARY_NAME) load -p $(POINTS) -w $(WORKERS)

load-1m: build
	@echo "Loading 1 million points..."
	./$(BINARY_NAME) load -p 1000000 -w $(WORKERS)

load-10m: build
	@echo "Loading 10 million points..."
	./$(BINARY_NAME) load -p 10000000 -w $(WORKERS)

load-100m: build
	@echo "Loading 100 million points..."
	./$(BINARY_NAME) load -p 100000000 -w $(WORKERS)

# Benchmark commands
benchmark: build check-index
	@echo "Running bounding box benchmark with $(QUERIES) queries using $(WORKERS) workers..."
	./$(BINARY_NAME) query -q $(QUERIES) -w $(WORKERS)

bench-radius: build check-index
	@echo "Running radius search benchmark ($(RADIUS)km) with $(QUERIES) queries using $(WORKERS) workers..."
	./$(BINARY_NAME) radius -q $(QUERIES) -r $(RADIUS) -w $(WORKERS)

bench-nearest: build check-index
	@echo "Running nearest neighbor benchmark (k=$(NEIGHBORS)) with $(QUERIES) queries using $(WORKERS) workers..."
	./$(BINARY_NAME) nearest -q $(QUERIES) -n $(NEIGHBORS) -w $(WORKERS)

bench-all: benchmark bench-radius bench-nearest
	@echo ""
	@echo "All benchmarks complete!"

# Performance testing with different configurations
perf-test: build
	@echo "Running performance tests with various configurations..."
	@echo ""
	@echo "=== Testing with 1M points ==="
	@$(MAKE) load-1m
	@$(MAKE) benchmark QUERIES=10000
	@echo ""
	@echo "=== Testing with 10M points ==="
	@$(MAKE) load-10m
	@$(MAKE) benchmark QUERIES=10000
	@echo ""
	@echo "=== Testing worker scaling ==="
	@echo "Testing with 1 worker..."
	@$(MAKE) benchmark QUERIES=1000 WORKERS=1
	@echo "Testing with 2 workers..."
	@$(MAKE) benchmark QUERIES=1000 WORKERS=2
	@echo "Testing with 4 workers..."
	@$(MAKE) benchmark QUERIES=1000 WORKERS=4
	@echo "Testing with 8 workers..."
	@$(MAKE) benchmark QUERIES=1000 WORKERS=8

# Utility targets
check-index:
	@if [ ! -f $(INDEX_FILE) ]; then \
		echo "Index file not found. Running 'make load' first..."; \
		$(MAKE) load; \
	fi

stats: build check-index
	@echo "Index statistics:"
	@ls -lh $(INDEX_FILE) | awk '{print "Index file size: " $$5}'
	@echo "Points in index: Check with the tool"

.SILENT: help