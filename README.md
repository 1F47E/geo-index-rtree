# Go Geo-Index

A high-performance R-Tree geographical indexing system written in Go, featuring parallel processing and PostGIS comparison benchmarks.

![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-green)

## 🚀 Features

- **High-Performance R-Tree Index**: In-memory spatial indexing with sub-microsecond query times
- **Parallel Processing**: Utilizes all CPU cores for maximum performance
- **PostGIS Comparison**: Built-in benchmarks against PostgreSQL/PostGIS
- **Beautiful CLI**: Colorful terminal output with real-time progress bars
- **Data Persistence**: Caches indexes for instant startup
- **Multiple Query Types**: Bounding box, radius search, and k-nearest neighbors

## 📊 Performance

Single-threaded benchmark on a 10-core system with 1 million geographic points:

| Operation | R-Tree | PostGIS | Speedup |
|-----------|--------|---------|---------|
| Single Query | ~2.2µs | ~24µs | **11x faster** |
| Queries/second | ~450,000 | ~40,000 | **11x faster** |
| Index Size | 42 MB | 65 MB | **35% smaller** |

**Key advantage**: R-Tree internally parallelizes each query across CPU partitions

## 🛠️ Installation

### Prerequisites

- Go 1.21 or higher
- Docker (for PostGIS comparison)
- Make

### Quick Start

```bash
# Clone the repository
git clone https://github.com/1F47E/geo-index-rtree.git
cd geo-index-rtree

# Install dependencies
make install-deps

# Run the demo
make demo
```

## 🎮 Usage

### Basic Demo (R-Tree Only)

```bash
make demo
```

This runs the R-Tree demo with colorful output showing:
- Loading 1 million random geographic points
- Running bounding box queries for 10 seconds
- Displaying performance metrics

### Full Comparison Demo

```bash
make demo-full
```

This runs both R-Tree and PostGIS benchmarks:
1. Starts PostGIS in Docker
2. Loads 1 million points into both systems
3. Runs identical benchmarks
4. Shows side-by-side comparison
5. Automatically stops PostGIS when done

### Real-World Cloud Demo

```bash
make demo-full-real
```

This simulates real-world cloud database performance:
- Adds network latency to PostGIS queries (configured in config.yaml)
- Default: 3ms latency (typical same-region cloud database)
- Shows the true advantage of in-memory R-Tree vs remote databases

### Example Output

```
🌍 Go Geo-Index Demo
============================================================

Using Existing Index
✓ Found existing index: geo_index.gob

  Index file size: 41.23 MB
  Points indexed: 1000000
  Points per MB: 24271
  CPU partitions: 10

Running R-Tree Bounding Box Queries
Running single-threaded benchmark for 10s
R-Tree advantage: Each query internally uses 10 CPU cores
✓ R-Tree Bounding Box Queries Complete!
  Total queries: 4512640
  Queries per second: 451264
  Average query time: 2.216µs
• Each query internally searched 10 partitions in parallel

Running PostGIS Bounding Box Queries
Running single-threaded benchmark for 10s
PostGIS: Each query runs sequentially (no internal parallelism)
✓ Found existing PostGIS data with 1000000 points

  Database size: 65 MB
  Table size: 42 MB
  Index size: 21 MB
  Points indexed: 1000000

✓ PostGIS Bounding Box Queries Complete!
  Total queries: 423510
  Queries per second: 42351
  Average query time: 23.612µs
• Each query executed sequentially without parallelism

Performance Comparison

Single-Threaded Benchmark Results:
• R-Tree: Each query internally parallelized across 10 CPU partitions
• PostGIS: Each query runs sequentially without parallelism

Metric               R-Tree (Internal Parallel)   PostGIS (Sequential)
--------------------------------------------------------------------------------
Queries/second       451264                       42351
Avg query time       2.216µs                      23.612µs
Total queries        4512640                      423510

R-Tree is 10.7x faster than PostGIS
This speedup comes from internal parallel execution across 10 CPU partitions
Both benchmarks used single-threaded query generation for fair comparison
```

## 🔧 Commands

### Demo Commands
- `make demo` - Run R-Tree demo only
- `make demo-full` - Run full comparison with PostGIS

### PostGIS Management
- `make postgis-up` - Start PostGIS container
- `make postgis-down` - Stop PostGIS container
- `make postgis-reset` - Clear PostGIS data and restart
- `make postgis-logs` - View PostGIS logs

### Data Management
- `make clean` - Remove binaries
- `make clean-cache` - Clear all cached data (R-Tree and PostGIS)

### Development
- `make build` - Build the binary
- `make test` - Run tests
- `make install-deps` - Install Go dependencies

### Loading Data
```bash
# Load specific number of points
make load POINTS=5000000 WORKERS=16

# Pre-configured loads
make load-1m    # 1 million points
make load-10m   # 10 million points
make load-100m  # 100 million points
```

### Running Benchmarks
```bash
# Bounding box queries
make benchmark QUERIES=10000 WORKERS=8

# Radius searches (50km default)
make bench-radius RADIUS=100

# Nearest neighbor searches
make bench-nearest NEIGHBORS=20

# Run all benchmarks
make bench-all
```

## 🏗️ Architecture

### Project Structure
```
go-geo-index/
├── cmd/
│   ├── main.go         # CLI entry point
│   └── demo/           # Demo application
├── pkg/
│   ├── rtree/          # R-Tree implementation
│   ├── postgis/        # PostGIS integration
│   └── models/         # Data models
├── data/
│   └── postgis/        # Persistent PostGIS data
└── docker-compose.yml  # PostGIS setup
```

### Key Components

1. **R-Tree Index** (`pkg/rtree/`)
   - Thread-safe with read-write locks
   - Supports concurrent queries
   - Persistent storage via GOB encoding

2. **PostGIS Integration** (`pkg/postgis/`)
   - Docker-based setup on port 5433
   - Connection pooling for performance
   - Bulk insert optimization

3. **Demo Application** (`cmd/demo/`)
   - Beautiful CLI with lipgloss styling
   - Real-time progress tracking
   - Automatic benchmark comparison

## ⚙️ Configuration

### Configuration File (config.yaml)

All demo parameters are configured in `config.yaml`:

```yaml
demo:
  points: 1000000              # Number of points to generate
  benchmark_duration: 10       # Benchmark duration in seconds

postgis:
  host: localhost
  port: 5499                   # PostGIS port
  user: geouser
  password: geopass
  database: geodb

network:
  simulated_latency_ms: 3      # Network latency simulation (0 = disabled)
```

### Environment Variables (Makefile)
- `POINTS` - Number of points to load (default: 1,000,000)
- `WORKERS` - Number of worker threads (default: CPU count)
- `QUERIES` - Number of queries to run (default: 1,000)
- `RADIUS` - Search radius in km (default: 50)
- `NEIGHBORS` - Number of nearest neighbors (default: 10)

## 🧪 Advanced Usage

### Direct CLI Usage
```bash
# Build the binary
make build

# Load points
./go-geo-index load -p 1000000 -w 8

# Run queries
./go-geo-index query -q 10000 -w 8

# Radius search
./go-geo-index radius -q 1000 -r 50 -w 8

# Nearest neighbors
./go-geo-index nearest -q 1000 -n 10 -w 8
```

### Performance Testing
```bash
# Test with various configurations
make perf-test

# This tests:
# - Different data sizes (1M, 10M points)
# - Worker scaling (1, 2, 4, 8 workers)
# - Various query types
```

## 🔍 Implementation Details

### R-Tree Features
- Uses [dhconnelly/rtreego](https://github.com/dhconnelly/rtreego) library
- Configurable tree parameters (min/max children)
- Efficient spatial pruning
- GOB serialization for persistence

### Parallel Processing
- **Point Generation**: Fully parallel across all cores
- **Index Building**: Currently sequential (mutex-protected)
- **Query Execution**: Fully parallel with read locks
- **Atomic Counters**: Thread-safe statistics

### PostGIS Integration
- GIST spatial indexing
- Connection pooling (25 connections)
- Bulk inserts (10k batch size)
- Prepared statements for performance

## 📈 Benchmarking Methodology

Each benchmark:
- Runs for exactly 10 seconds
- Uses all available CPU cores
- Generates random queries
- Measures queries per second and latency

Query parameters:
- **Bounding boxes**: 0.1° to 2.0° in size
- **Radius searches**: Default 50km radius
- **Nearest neighbors**: Default k=10

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Development Setup
```bash
# Clone and setup
git clone https://github.com/1F47E/geo-index-rtree.git
cd geo-index-rtree
make install-deps

# Run tests
make test

# Run with verbose output
./go-geo-index load -v
```

## 📝 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- [dhconnelly/rtreego](https://github.com/dhconnelly/rtreego) - R-Tree implementation
- [PostGIS](https://postgis.net/) - Spatial database extension
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Style definitions
- [Cobra](https://github.com/spf13/cobra) - CLI framework

## 🚧 Known Limitations

1. **R-Tree insertion is sequential** - Due to mutex locking, only one goroutine can insert at a time
2. **PostGIS on ARM Macs** - Uses x86 emulation which may impact performance
3. **Memory usage** - Entire index is kept in memory, ~42MB per million points

## 🔮 Future Improvements

- [ ] Parallel R-Tree construction
- [ ] Support for polygons and linestrings  
- [ ] Web-based visualization dashboard
- [ ] Distributed index support
- [ ] Custom spatial reference systems
- [ ] Streaming updates support
- [ ] Index compression options