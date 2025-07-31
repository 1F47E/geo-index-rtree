# Go Geo-Index Project Memory

## Project Overview
High-performance R-Tree geographical indexing demo in Go, with PostGIS comparison benchmarks.

## Key Features
- R-Tree implementation using dhconnelly/rtreego with CPU-aware partitioning
- Parallel single-query execution (each query uses all CPU cores internally)
- Single-threaded benchmarking for fair comparison
- PostGIS integration for performance comparison
- Colorful terminal UI with progress bars
- Persistent data caching for both R-tree and PostGIS

## Architecture

### Core Components
1. **R-Tree Index** (`pkg/rtree/`)
   - CPU-aware partitioned architecture (one tree per CPU core)
   - Parallel single-query execution across all partitions
   - Spatial partitioning based on longitude bands
   - Thread-safe implementation with RWMutex
   - Supports bounding box queries, radius search, and k-nearest neighbors
   - Persistence via GOB encoding

2. **PostGIS Integration** (`pkg/postgis/`)
   - Docker-based PostGIS setup
   - GIST spatial indexing
   - Connection pooling (25 connections)
   - Bulk insert optimization (10k batch size)

3. **Demo Application** (`cmd/demo/demo.go`)
   - Loads 1 million random points distributed across CPU partitions
   - Runs 10-second single-threaded benchmarks
   - Shows side-by-side performance comparison
   - Clear messaging about internal parallelism vs sequential execution
   - Auto-stops PostGIS container when done

## Performance Characteristics

### Multi-Core Scaling
- **Point Generation**: Fully parallel across all CPU cores
- **R-Tree Insertion**: Distributed across partitions based on longitude
- **R-Tree Single Query**: Executes in parallel across all CPU partitions
- **R-Tree Concurrent Queries**: Multiple queries run concurrently
- **PostGIS**: Sequential single queries, parallel via connection pool

### Typical Results (M1 Mac, 10 cores, single-threaded benchmark)
- R-Tree: ~450,000 queries/second (each query internally uses all 10 cores)
- PostGIS: ~40,000 queries/second (each query runs sequentially)
- R-Tree is typically 10-15x faster due to internal parallel execution
- Average single query latency: R-Tree ~2.2µs, PostGIS ~24µs

## Data Persistence

### R-Tree
- Stored in `geo_index.gob` file
- Checked on startup, skips loading if 1M+ points exist
- Shows file size, point count, points/MB ratio

### PostGIS
- Data persisted in `./data/postgis/` directory
- Survives container restarts
- Shows database size, table size, index size when reused

## Commands

### Basic Demo
```bash
make demo           # R-tree only
make demo-full      # R-tree + PostGIS comparison
```

### PostGIS Management
```bash
make postgis-up     # Start PostGIS container (port 5433)
make postgis-down   # Stop container (auto-done after demo)
make postgis-reset  # Clear PostGIS data
make postgis-logs   # View PostGIS logs
```

### Cache Management
```bash
make clean-cache    # Clear all cached data (forces reload)
```

## Configuration

### Docker Setup
- PostGIS runs on port 5499 (avoids conflicts)
- Platform: linux/amd64 (works on ARM via emulation)
- Credentials: geouser/geopass/geodb

### Adjustable Parameters
- Point count: Change `numPoints` in `loadAndIndex()` (default: 1M)
- Benchmark duration: Change `benchDuration` (default: 10s)
- Partition count: Uses `runtime.NumCPU()` automatically
- Both benchmarks run single-threaded for fair comparison

## Known Limitations
1. PostGIS on ARM Macs uses x86 emulation (slower)
2. No tree statistics exposed by rtreego library
3. Partition rebalancing not implemented (fixed longitude bands)
4. Nearest neighbor queries currently fetch more candidates than needed from each partition

## Implementation Details

### R-Tree Partitioning Strategy
- Points are distributed across partitions based on longitude
- Each partition covers 360°/numCPU degrees of longitude
- Example: 10 cores = 36° per partition
- Queries determine relevant partitions based on bounding box overlap
- All relevant partitions are searched in parallel

### Benchmark Methodology
- Single-threaded query generation ensures fair comparison
- R-tree: Each query spawns numCPU goroutines internally
- PostGIS: Each query executes on a single connection
- This isolates the algorithmic advantage of parallel execution
- Results show pure performance difference, not concurrency effects

## Recent Updates
- **MAJOR**: Implemented CPU-aware R-tree partitioning for parallel query execution
- Each R-tree query now executes in parallel across all CPU cores internally
- Changed to single-threaded benchmarking for fair comparison
- Updated demo messaging to clearly explain internal parallelism advantage
- Clear distinction between "parallel internally" (R-tree) vs "sequential" (PostGIS)
- Added PostGIS benchmark comparison
- Implemented persistent data storage
- Added colorful terminal output with lipgloss
- Auto-cleanup of PostGIS after benchmarks
- Database statistics display for existing data
- Port changed from 5432 to 5433 to 5499 to avoid conflicts