package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/1F47E/geo-index-rtree/pkg/models"
	"github.com/1F47E/geo-index-rtree/pkg/postgis"
	"github.com/1F47E/geo-index-rtree/pkg/rtree"
	"github.com/mattn/go-isatty"
	"gopkg.in/yaml.v3"
)

const indexFile = "geo_index.gob"

// Config structure for YAML configuration
type Config struct {
	Demo struct {
		Points             int `yaml:"points"`
		BenchmarkDuration  int `yaml:"benchmark_duration"`
	} `yaml:"demo"`
	PostGIS struct {
		Host               string `yaml:"host"`
		Port               int    `yaml:"port"`
		User               string `yaml:"user"`
		Password           string `yaml:"password"`
		Database           string `yaml:"database"`
		MaxConnections     int    `yaml:"max_connections"`
		ConnectionTimeout  int    `yaml:"connection_timeout"`
	} `yaml:"postgis"`
	Network struct {
		SimulatedLatencyMs int `yaml:"simulated_latency_ms"`
	} `yaml:"network"`
}

var (
	// ANSI color codes
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
	
	// Configuration
	config Config
	
	// Network latency simulation
	simulateNetworkLatency = false
	networkLatency time.Duration
)

func init() {
	// Disable colors if not in a terminal
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		colorReset = ""
		colorRed = ""
		colorGreen = ""
		colorYellow = ""
		colorBlue = ""
		colorPurple = ""
		colorCyan = ""
		colorBold = ""
	}
}

func printTitle(title string) {
	fmt.Printf("\n%s%s🌍 %s%s\n", colorBold, colorPurple, title, colorReset)
	fmt.Println(strings.Repeat("=", 60))
}

func printSubtitle(subtitle string) {
	fmt.Printf("\n%s%s%s%s\n", colorBold, colorCyan, subtitle, colorReset)
}

func printSuccess(message string) {
	fmt.Printf("%s✓ %s%s\n", colorGreen, message, colorReset)
}

func printInfo(message string) {
	fmt.Printf("%s• %s%s\n", colorYellow, message, colorReset)
}

func printStat(label string, value interface{}) {
	fmt.Printf("  %s%s:%s %s%v%s\n", colorBold, label, colorReset, colorYellow, value, colorReset)
}

func printProgress(current, total int, label string) {
	percent := float64(current) / float64(total) * 100
	barLength := 40
	filled := int(percent / 100 * float64(barLength))
	
	bar := "["
	for i := 0; i < barLength; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	bar += "]"
	
	fmt.Printf("\r%s %s%.1f%%%s %s", label, colorCyan, percent, colorReset, bar)
	if current >= total {
		fmt.Println()
	}
}

func loadConfig() error {
	// Try to load config.yaml
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		// If config.yaml doesn't exist, try config.yaml.example
		data, err = os.ReadFile("config.yaml.example")
		if err != nil {
			return fmt.Errorf("config.yaml not found. Please copy config.yaml.example to config.yaml")
		}
		fmt.Printf("%sUsing config.yaml.example (copy to config.yaml for custom settings)%s\n", colorYellow, colorReset)
	}
	
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	
	return nil
}

func main() {
	// Load configuration
	if err := loadConfig(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	
	// Check for network latency simulation flag
	if len(os.Args) > 1 && os.Args[1] == "--network-latency" {
		simulateNetworkLatency = true
		networkLatency = time.Duration(config.Network.SimulatedLatencyMs) * time.Millisecond
	}
	
	printTitle("Go Geo-Index Demo")
	
	// Phase 1: Load Points
	loadAndIndex()
	
	// Phase 2: R-Tree Bounding Box Queries
	time.Sleep(500 * time.Millisecond)
	rtreeStats := runBenchmarks()
	
	// Phase 3: PostGIS Bounding Box Queries
	time.Sleep(500 * time.Millisecond)
	postgisStats := runPostGISBenchmark()
	
	// Phase 4: Radius Searches
	// time.Sleep(500 * time.Millisecond)
	// runRadiusSearches()
	
	// Phase 5: Nearest Neighbor
	// time.Sleep(500 * time.Millisecond)
	// runNearestNeighbors()
	
	// Summary
	printComparison(rtreeStats, postgisStats)
	printSummary()
	
	// Stop PostGIS if it was used
	if postgisStats.totalQueries > 0 {
		fmt.Println()
		printInfo("Stopping PostGIS container...")
		cmd := exec.Command("docker", "compose", "down")
		if err := cmd.Run(); err != nil {
			printError("Failed to stop PostGIS container. Run 'make postgis-down' manually.")
		} else {
			printSuccess("PostGIS container stopped")
		}
	}
}

func loadAndIndex() {
	// Check if index already exists
	if fileInfo, err := os.Stat(indexFile); err == nil {
		printSubtitle("Using Existing Index")
		
		// Load and verify the index
		index := rtree.NewGeoIndex()
		if err := index.LoadFromFile(indexFile); err != nil {
			fmt.Printf("%sError loading existing index: %v%s\n", colorRed, err, colorReset)
			fmt.Println("Regenerating index...")
		} else {
			count := index.Count()
			
			// Get file size in human readable format
			fileSize := fileInfo.Size()
			var sizeStr string
			switch {
			case fileSize >= 1<<30: // GB
				sizeStr = fmt.Sprintf("%.2f GB", float64(fileSize)/(1<<30))
			case fileSize >= 1<<20: // MB
				sizeStr = fmt.Sprintf("%.2f MB", float64(fileSize)/(1<<20))
			case fileSize >= 1<<10: // KB
				sizeStr = fmt.Sprintf("%.2f KB", float64(fileSize)/(1<<10))
			default:
				sizeStr = fmt.Sprintf("%d bytes", fileSize)
			}
			
			printSuccess(fmt.Sprintf("Found existing index: %s", indexFile))
			fmt.Println()
			printStat("Index file size", sizeStr)
			printStat("Points indexed", fmt.Sprintf("%s%d%s", colorGreen, count, colorReset))
			printStat("Points per MB", fmt.Sprintf("%.0f", float64(count)/(float64(fileSize)/(1<<20))))
			printStat("CPU partitions", runtime.NumCPU())
			
			if count >= int64(config.Demo.Points) {
				fmt.Println()
				printInfo("Skipping index generation - using existing data")
				return
			}
			fmt.Printf("\n%sExisting index has insufficient points, regenerating...%s\n", colorYellow, colorReset)
		}
	}
	
	printSubtitle("Loading Points")
	
	numPoints := config.Demo.Points
	numCPU := runtime.NumCPU()
	
	fmt.Printf("Generating %s%d%s random geographic points...\n", colorBold, numPoints, colorReset)
	fmt.Printf("System has %s%d CPU cores%s available\n", colorBold, numCPU, colorReset)
	fmt.Printf("Points will be distributed across %s%d spatial partitions%s (one per CPU)\n", 
		colorBold, numCPU, colorReset)
	
	// Generate points
	points := generateRandomPoints(numPoints)
	
	// Create index
	index := rtree.NewGeoIndex()
	
	start := time.Now()
	
	// Load all points into the partitioned index
	var loaded atomic.Int32
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				printProgress(numPoints, numPoints, "Indexing")
				return
			case <-ticker.C:
				// Simulate progress for user feedback
				current := int(loaded.Load())
				if current < numPoints {
					loaded.Add(int32(numPoints / 100)) // Increment by 1%
					if loaded.Load() > int32(numPoints) {
						loaded.Store(int32(numPoints))
					}
				}
				printProgress(int(loaded.Load()), numPoints, "Indexing")
			}
		}
	}()
	
	// Index all points at once - the IndexPoints method handles partitioning internally
	err := index.IndexPoints(points)
	if err != nil {
		log.Printf("Error indexing points: %v", err)
	}
	loaded.Store(int32(numPoints))
	done <- true
	loadTime := time.Since(start)
	
	// Save index
	if err := index.SaveToFile(indexFile); err != nil {
		log.Printf("Error saving index: %v", err)
	}
	
	fmt.Println()
	printSuccess(fmt.Sprintf("Indexed %d points across %d partitions in %v", numPoints, numCPU, loadTime))
	printSuccess(fmt.Sprintf("Indexing rate: %.0f points/second", float64(numPoints)/loadTime.Seconds()))
	printSuccess(fmt.Sprintf("Index saved to %s", indexFile))
}

type benchmarkStats struct {
	queriesPerSecond float64
	avgQueryTime     time.Duration
	totalQueries     int64
}

func runBenchmarks() benchmarkStats {
	printSubtitle("Running R-Tree Bounding Box Queries")
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	benchDuration := time.Duration(config.Demo.BenchmarkDuration) * time.Second
	
	fmt.Printf("Running %ssingle-threaded%s benchmark for %s%v%s\n", 
		colorBold, colorReset, colorBold, benchDuration, colorReset)
	fmt.Printf("R-Tree advantage: Each query %sinternally uses %d CPU cores%s\n",
		colorGreen, runtime.NumCPU(), colorReset)
	
	// Run benchmark
	var queryCount atomic.Int64
	
	start := time.Now()
	deadline := start.Add(benchDuration)
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				fmt.Println()
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				percent := elapsed.Seconds() / benchDuration.Seconds() * 100
				if percent > 100 {
					percent = 100
				}
				printProgress(int(percent), 100, "Benchmarking")
			}
		}
	}()
	
	// Single-threaded benchmark
	for time.Now().Before(deadline) {
		// Random query
		centerLat := rand.Float64()*180 - 90
		centerLon := rand.Float64()*360 - 180
		boxSize := rand.Float64()*1.9 + 0.1
		
		box := models.BoundingBox{
			BottomLeft: models.Location{Lat: centerLat - boxSize/2, Lon: centerLon - boxSize/2},
			TopRight: models.Location{Lat: centerLat + boxSize/2, Lon: centerLon + boxSize/2},
		}
		
		_, err := index.QueryBox(box)
		if err == nil {
			queryCount.Add(1)
		}
	}
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("R-Tree Bounding Box Queries Complete!")
	printStat("Total queries", fmt.Sprintf("%d", completedQueries))
	printStat("Queries per second", fmt.Sprintf("%s%.0f%s", colorGreen, float64(completedQueries)/elapsed.Seconds(), colorReset))
	printStat("Average query time", fmt.Sprintf("%s%v%s", colorGreen, elapsed/time.Duration(completedQueries), colorReset))
	printInfo(fmt.Sprintf("Each query internally searched %d partitions in parallel", runtime.NumCPU()))
	
	return benchmarkStats{
		queriesPerSecond: float64(completedQueries)/elapsed.Seconds(),
		avgQueryTime:     elapsed/time.Duration(completedQueries),
		totalQueries:     completedQueries,
	}
}

func runPostGISBenchmark() benchmarkStats {
	printSubtitle("Running PostGIS Bounding Box Queries")
	
	// Connect to PostGIS
	printInfo("Connecting to PostGIS...")
	db, err := postgis.NewPostGISIndex(
		config.PostGIS.Host, 
		config.PostGIS.User, 
		config.PostGIS.Password, 
		config.PostGIS.Database, 
		config.PostGIS.Port)
	if err != nil {
		printError(fmt.Sprintf("PostGIS connection failed: %v", err))
		fmt.Println()
		printInfo("Skipping PostGIS benchmark. To enable PostGIS:")
		printInfo("1. Ensure Docker is running")
		printInfo("2. Run 'make postgis-up' to start PostGIS")
		printInfo("3. If data is corrupted, run 'make clean-cache' first")
		fmt.Println()
		return benchmarkStats{}
	}
	defer db.Close()
	printSuccess("Connected to PostGIS")
	
	// Check if data is already loaded
	count, err := db.Count()
	if err == nil && count >= int64(config.Demo.Points) {
		printSuccess(fmt.Sprintf("Found existing PostGIS data with %d points", count))
		
		// Get and display database statistics
		stats, err := db.GetDatabaseStats()
		if err == nil {
			fmt.Println()
			printStat("Database size", stats["database_size"])
			printStat("Table size", stats["table_size"])
			printStat("Index size", stats["index_size"])
			printStat("Points indexed", fmt.Sprintf("%s%d%s", colorGreen, stats["row_count"], colorReset))
			fmt.Println()
		}
	} else {
		// Load data into PostGIS
		printInfo("Loading points into PostGIS...")
		
		// Initialize schema
		if err := db.InitSchema(); err != nil {
			log.Printf("Failed to initialize schema: %v", err)
			return benchmarkStats{}
		}
		
		// Generate same points
		points := generateRandomPoints(config.Demo.Points)
		
		// Bulk insert with progress
		start := time.Now()
		lastProgress := 0
		
		progressCallback := func(loaded, total int) {
			percent := loaded * 100 / total
			if percent > lastProgress {
				printProgress(percent, 100, fmt.Sprintf("Loading %d points", total))
				lastProgress = percent
			}
		}
		
		fmt.Println() // New line for progress bar
		err = db.BulkInsertPoints(points, progressCallback)
		fmt.Println() // Clear line after progress
		
		if err != nil {
			log.Printf("Failed to insert points: %v", err)
			return benchmarkStats{}
		}
		
		loadElapsed := time.Since(start)
		printSuccess(fmt.Sprintf("Loaded %d points in %v", len(points), loadElapsed))
		
		// Create spatial index
		printInfo("Creating spatial index...")
		indexStart := time.Now()
		if err := db.CreateSpatialIndex(); err != nil {
			log.Printf("Failed to create spatial index: %v", err)
			return benchmarkStats{}
		}
		indexElapsed := time.Since(indexStart)
		printSuccess(fmt.Sprintf("Created spatial index in %v", indexElapsed))
		
		totalElapsed := time.Since(start)
		fmt.Println()
		printStat("Total PostGIS setup time", totalElapsed.String())
	}
	
	benchDuration := time.Duration(config.Demo.BenchmarkDuration) * time.Second
	
	fmt.Printf("Running %ssingle-threaded%s benchmark for %s%v%s\n", 
		colorBold, colorReset, colorBold, benchDuration, colorReset)
	fmt.Printf("PostGIS: Each query runs %ssequentially%s (no internal parallelism)\n", colorYellow, colorReset)
	if simulateNetworkLatency {
		fmt.Printf("%sSimulating network latency: +%v per query%s\n", colorCyan, networkLatency, colorReset)
	}
	
	// Run benchmark
	var queryCount atomic.Int64
	
	start := time.Now()
	deadline := start.Add(benchDuration)
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				fmt.Println()
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				percent := elapsed.Seconds() / benchDuration.Seconds() * 100
				if percent > 100 {
					percent = 100
				}
				printProgress(int(percent), 100, "Benchmarking")
			}
		}
	}()
	
	// Single-threaded benchmark
	for time.Now().Before(deadline) {
		// Random query
		centerLat := rand.Float64()*180 - 90
		centerLon := rand.Float64()*360 - 180
		boxSize := rand.Float64()*1.9 + 0.1
		
		box := models.BoundingBox{
			BottomLeft: models.Location{Lat: centerLat - boxSize/2, Lon: centerLon - boxSize/2},
			TopRight: models.Location{Lat: centerLat + boxSize/2, Lon: centerLon + boxSize/2},
		}
		
		_, err := db.QueryBox(box)
		if err == nil {
			queryCount.Add(1)
			// Simulate network latency
			if simulateNetworkLatency {
				time.Sleep(networkLatency)
			}
		}
	}
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("PostGIS Bounding Box Queries Complete!")
	printStat("Total queries", fmt.Sprintf("%d", completedQueries))
	printStat("Queries per second", fmt.Sprintf("%s%.0f%s", colorYellow, float64(completedQueries)/elapsed.Seconds(), colorReset))
	printStat("Average query time", fmt.Sprintf("%s%v%s", colorYellow, elapsed/time.Duration(completedQueries), colorReset))
	if simulateNetworkLatency {
		printInfo(fmt.Sprintf("Each query included %v simulated network latency", networkLatency))
	} else {
		printInfo("Each query executed sequentially without parallelism")
	}
	
	return benchmarkStats{
		queriesPerSecond: float64(completedQueries)/elapsed.Seconds(),
		avgQueryTime:     elapsed/time.Duration(completedQueries),
		totalQueries:     completedQueries,
	}
}

func printError(message string) {
	fmt.Printf("%s✗ %s%s\n", colorRed, message, colorReset)
}

func printComparison(rtreeStats, postgisStats benchmarkStats) {
	printTitle("Performance Comparison")
	
	fmt.Printf("\n%sSingle-Threaded Benchmark Results:%s\n", colorBold, colorReset)
	fmt.Printf("• R-Tree: Each query %sinternally parallelized%s across %d CPU partitions\n", colorGreen, colorReset, runtime.NumCPU())
	if simulateNetworkLatency {
		fmt.Printf("• PostGIS: Each query runs %ssequentially%s with %s%v network latency%s\n\n", 
			colorYellow, colorReset, colorCyan, networkLatency, colorReset)
	} else {
		fmt.Printf("• PostGIS: Each query runs %ssequentially%s without parallelism\n\n", colorYellow, colorReset)
	}
	
	fmt.Printf("%s%-20s %-30s %-30s%s\n", colorBold, "Metric", "R-Tree (Internal Parallel)", "PostGIS (Sequential)", colorReset)
	fmt.Println(strings.Repeat("-", 80))
	
	// Queries per second
	rtreeQPS := fmt.Sprintf("%.0f", rtreeStats.queriesPerSecond)
	postgisQPS := "N/A"
	if postgisStats.queriesPerSecond > 0 {
		postgisQPS = fmt.Sprintf("%.0f", postgisStats.queriesPerSecond)
	}
	fmt.Printf("%-20s %s%-30s%s %s%-30s%s\n", "Queries/second", 
		colorGreen, rtreeQPS, colorReset,
		colorYellow, postgisQPS, colorReset)
	
	// Average query time
	rtreeAvg := rtreeStats.avgQueryTime.String()
	postgisAvg := "N/A"
	if postgisStats.avgQueryTime > 0 {
		postgisAvg = postgisStats.avgQueryTime.String()
	}
	fmt.Printf("%-20s %s%-30s%s %s%-30s%s\n", "Avg query time",
		colorGreen, rtreeAvg, colorReset,
		colorYellow, postgisAvg, colorReset)
	
	// Total queries
	fmt.Printf("%-20s %-30d", "Total queries", rtreeStats.totalQueries)
	if postgisStats.totalQueries > 0 {
		fmt.Printf(" %-30d\n", postgisStats.totalQueries)
	} else {
		fmt.Printf(" %-30s\n", "N/A")
	}
	
	// Performance ratio
	if postgisStats.queriesPerSecond > 0 {
		ratio := rtreeStats.queriesPerSecond / postgisStats.queriesPerSecond
		fmt.Printf("\n%sR-Tree is %.1fx faster than PostGIS%s\n", colorBold, ratio, colorReset)
		if simulateNetworkLatency {
			fmt.Printf("This represents %sreal-world cloud/remote database performance%s\n", 
				colorCyan, colorReset)
			fmt.Printf("R-Tree advantage: %sNo network overhead%s for in-memory queries\n", 
				colorGreen, colorReset)
		} else {
			fmt.Printf("This speedup comes from %sinternal parallel execution%s across %d CPU partitions\n", 
				colorGreen, colorReset, runtime.NumCPU())
			fmt.Printf("Both benchmarks used %ssingle-threaded%s query generation for fair comparison\n", 
				colorBold, colorReset)
		}
	}
	
	fmt.Println()
}

func runRadiusSearches() {
	printSubtitle("Running Radius Searches")
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	benchDuration := 10 * time.Second
	numWorkers := runtime.NumCPU()
	searchRadius := 50.0 // km
	
	fmt.Printf("Running radius searches (%s%.0f km%s) for %s%v%s using %s%d%s workers...\n",
		colorBold, searchRadius, colorReset,
		colorBold, benchDuration, colorReset, 
		colorBold, numWorkers, colorReset)
	
	// Run benchmark
	var queryCount atomic.Int64
	
	start := time.Now()
	deadline := start.Add(benchDuration)
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				fmt.Println()
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				percent := elapsed.Seconds() / benchDuration.Seconds() * 100
				if percent > 100 {
					percent = 100
				}
				printProgress(int(percent), 100, "Benchmarking")
			}
		}
	}()
	
	var wg sync.WaitGroup
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		
		go func() {
			defer wg.Done()
			
			for time.Now().Before(deadline) {
				// Random center point
				center := models.Location{
					Lat: rand.Float64()*180 - 90,
					Lon: rand.Float64()*360 - 180,
				}
				
				_, err := index.QueryRadius(center, searchRadius)
				if err == nil {
					queryCount.Add(1)
				}
			}
		}()
	}
	
	wg.Wait()
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("Radius Searches Complete!")
	printStat("Queries per second", fmt.Sprintf("%s%.0f%s", colorGreen, float64(completedQueries)/elapsed.Seconds(), colorReset))
	printStat("Average query time", fmt.Sprintf("%s%v%s", colorGreen, elapsed/time.Duration(completedQueries), colorReset))
}

func runNearestNeighbors() {
	printSubtitle("Running Nearest Neighbor Searches")
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	benchDuration := 10 * time.Second
	numWorkers := runtime.NumCPU()
	numNeighbors := 10
	
	fmt.Printf("Finding %s%d%s nearest neighbors for %s%v%s using %s%d%s workers...\n",
		colorBold, numNeighbors, colorReset,
		colorBold, benchDuration, colorReset,
		colorBold, numWorkers, colorReset)
	
	// Run benchmark
	var queryCount atomic.Int64
	
	start := time.Now()
	deadline := start.Add(benchDuration)
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				fmt.Println()
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				percent := elapsed.Seconds() / benchDuration.Seconds() * 100
				if percent > 100 {
					percent = 100
				}
				printProgress(int(percent), 100, "Benchmarking")
			}
		}
	}()
	
	var wg sync.WaitGroup
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		
		go func() {
			defer wg.Done()
			
			for time.Now().Before(deadline) {
				// Random query point
				center := models.Location{
					Lat: rand.Float64()*180 - 90,
					Lon: rand.Float64()*360 - 180,
				}
				
				_ = index.NearestNeighbors(center, numNeighbors)
				queryCount.Add(1)
			}
		}()
	}
	
	wg.Wait()
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("Nearest Neighbor Searches Complete!")
	printStat("Queries per second", fmt.Sprintf("%s%.0f%s", colorGreen, float64(completedQueries)/elapsed.Seconds(), colorReset))
	printStat("Average query time", fmt.Sprintf("%s%v%s", colorGreen, elapsed/time.Duration(completedQueries), colorReset))
}

func printSummary() {
	printTitle("Demo Complete! 🎉")
	
	fmt.Printf("\n%sThe R-Tree index demonstrated:%s\n", colorBold, colorReset)
	printInfo(fmt.Sprintf("CPU-aware partitioning across %d cores", runtime.NumCPU()))
	printInfo(fmt.Sprintf("Internal parallel execution - each query searches %d partitions", runtime.NumCPU()))
	printInfo("In-memory spatial indexing with microsecond latency")
	printInfo("Single-threaded benchmark shows pure algorithmic advantage")
	
	fmt.Printf("\n%sBenchmark Duration:%s 10 seconds per test\n", colorBold, colorReset)
	fmt.Printf("%sTest Dataset:%s 1,000,000 geographic points\n", colorBold, colorReset)
	
	fmt.Println()
}

func generateRandomPoints(n int) []*models.Point {
	points := make([]*models.Point, n)
	
	numWorkers := runtime.NumCPU()
	batchSize := n / numWorkers
	var wg sync.WaitGroup
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * batchSize
		endIdx := startIdx + batchSize
		if w == numWorkers-1 {
			endIdx = n
		}
		
		go func(start, end int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(start)))
			
			for i := start; i < end; i++ {
				var lat, lon float64
				
				switch r.Intn(5) {
				case 0: // North America
					lat = r.Float64()*30 + 30
					lon = r.Float64()*60 - 120
				case 1: // Europe
					lat = r.Float64()*20 + 40
					lon = r.Float64()*40 - 10
				case 2: // Asia
					lat = r.Float64()*40 + 20
					lon = r.Float64()*80 + 60
				case 3: // South America
					lat = r.Float64()*40 - 50
					lon = r.Float64()*30 - 80
				default: // Random
					lat = r.Float64()*180 - 90
					lon = r.Float64()*360 - 180
				}
				
				points[i] = &models.Point{
					ID: fmt.Sprintf("point_%d", i),
					Location: &models.Location{
						Lat: lat,
						Lon: lon,
					},
				}
			}
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	return points
}

var strings = struct {
	Repeat func(string, int) string
}{
	Repeat: func(s string, n int) string {
		result := ""
		for i := 0; i < n; i++ {
			result += s
		}
		return result
	},
}