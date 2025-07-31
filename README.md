# Go Geo-Index: High-Performance R-Tree Implementation

A demonstration of R-Tree technology for efficient geo-spatial queries using Go's concurrency features. This project showcases maximum efficiency by utilizing goroutines to parallelize operations across all CPU cores.

## Features

- **Parallel Processing**: Utilizes all CPU cores for indexing and querying
- **R-Tree Index**: Efficient spatial indexing structure for geographic data
- **Multiple Query Types**:
  - Bounding box searches
  - Radius searches
  - K-nearest neighbor searches
- **Persistence**: Save and load indices from disk
- **Benchmarking**: Built-in performance testing tools

## Quick Start

```bash
# Install dependencies
make install-deps

# Build the project
make build

# Run the complete demo (loads 1M points and runs all benchmarks)
make demo
```

## Usage Examples

### Loading Data

```bash
# Load 1 million random points (default)
make load-1m

# Load 10 million points
make load-10m

# Load custom number of points
make load POINTS=5000000

# Load with specific number of workers
make load POINTS=1000000 WORKERS=16
```

### Running Benchmarks

```bash
# Run bounding box queries (default: 1000 queries)
make benchmark

# Run with custom parameters
make benchmark QUERIES=10000 WORKERS=8

# Run radius search benchmark (default: 50km radius)
make bench-radius RADIUS=100

# Run nearest neighbor benchmark
make bench-nearest NEIGHBORS=20

# Run all benchmarks
make bench-all
```

## Performance Characteristics

The R-Tree implementation provides:

- **O(log n)** average query time complexity
- **Parallel indexing** using all CPU cores
- **Efficient disk persistence** using Go's gob encoding
- **Thread-safe operations** for concurrent queries

## Architecture

```
go-geo-index/
├── cmd/
│   └── main.go          # CLI application
├── pkg/
│   └── geo/
│       └── rtree.go     # Core R-Tree implementation
├── Makefile             # Build and benchmark commands
├── go.mod               # Go module definition
└── README.md            # This file
```

## Implementation Details

### Parallel Indexing
Points are distributed across workers based on CPU count, with each worker processing its batch independently before insertion into the R-Tree.

### Query Types

1. **Bounding Box Search**: Find all points within a rectangular area
2. **Radius Search**: Find all points within a specific distance from a center point
3. **K-Nearest Neighbors**: Find the K closest points to a query location

### Optimization Techniques

- Goroutine pools for concurrent processing
- Atomic counters for thread-safe statistics
- Efficient memory allocation strategies
- Minimal lock contention through read-write mutex

## Benchmarking

Run performance tests to see the efficiency:

```bash
# Full performance test suite
make perf-test

# This will test:
# - Different data sizes (1M, 10M points)
# - Worker scaling (1, 2, 4, 8 workers)
# - Various query types
```

## Example Output

```
Loading 1000000 random points into R-Tree index using 8 workers...
Loaded 1000000 points in 1.2s
Points per second: 833333

Running 1000 bounding box queries using 8 workers...
Benchmark Results:
Total queries: 1000
Total time: 15ms
Queries per second: 66666
Average query time: 15µs
```

## Requirements

- Go 1.21 or later
- Multi-core CPU recommended for best performance

## License

This is a demonstration project showcasing R-Tree technology and Go's concurrency capabilities.