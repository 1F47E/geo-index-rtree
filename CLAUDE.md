# Go Geo-Index Project Memory

## Project Overview
High-performance R-Tree geographical indexing demo in Go, with PostGIS comparison benchmarks.

## Key Features
- R-Tree implementation using dhconnelly/rtreego
- Parallel point loading and querying
- PostGIS integration for performance comparison
- Colorful terminal UI with progress bars
- Persistent data caching for both R-tree and PostGIS

## Architecture

### Core Components
1. **R-Tree Index** (`pkg/rtree/`)
   - Thread-safe implementation with RWMutex
   - Supports bounding box queries, radius search, and k-nearest neighbors
   - Persistence via GOB encoding
   - Note: Tree insertion is mutex-locked (sequential), but queries are concurrent

2. **PostGIS Integration** (`pkg/postgis/`)
   - Docker-based PostGIS setup
   - GIST spatial indexing
   - Connection pooling (25 connections)
   - Bulk insert optimization (10k batch size)

3. **Demo Application** (`cmd/demo/demo.go`)
   - Loads 1 million random points
   - Runs 10-second benchmarks
   - Shows side-by-side performance comparison
   - Auto-stops PostGIS container when done

## Performance Characteristics

### Multi-Core Scaling
- **Point Generation**: Fully parallel across all CPU cores
- **R-Tree Insertion**: Sequential due to mutex (bottleneck)
- **R-Tree Queries**: Fully parallel with RLock
- **PostGIS**: Parallel queries via connection pool

### Typical Results (M1 Mac, 10 cores)
- R-Tree: ~450,000 queries/second
- PostGIS: ~40,000 queries/second
- R-Tree is typically 10-15x faster for bounding box queries

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
- PostGIS runs on port 5433 (avoids conflicts)
- Platform: linux/amd64 (works on ARM via emulation)
- Credentials: geouser/geopass/geodb

### Adjustable Parameters
- Point count: Change `numPoints` in `loadAndIndex()` (default: 1M)
- Benchmark duration: Change `benchDuration` (default: 10s)
- Worker count: Uses `runtime.NumCPU()` automatically

## Known Limitations
1. R-tree insertion is not parallel (mutex bottleneck)
2. PostGIS on ARM Macs uses x86 emulation (slower)
3. No tree statistics exposed by rtreego library

## Recent Updates
- Added PostGIS benchmark comparison
- Implemented persistent data storage
- Added colorful terminal output with lipgloss
- Auto-cleanup of PostGIS after benchmarks
- Database statistics display for existing data
- Port changed from 5432 to 5433 to avoid conflicts