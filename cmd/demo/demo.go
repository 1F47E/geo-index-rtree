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

	"github.com/kass/go-geo-index/pkg/models"
	"github.com/kass/go-geo-index/pkg/postgis"
	"github.com/kass/go-geo-index/pkg/rtree"
	"github.com/mattn/go-isatty"
)

const indexFile = "geo_index.gob"

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

func main() {
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
	
	// Stop PostGIS
	if postgisStats.totalQueries > 0 {
		printInfo("Stopping PostGIS container...")
		cmd := exec.Command("docker-compose", "down")
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
			printStat("Worker threads", runtime.NumCPU())
			
			if count >= 1000000 {
				fmt.Println()
				printInfo("Skipping index generation - using existing data")
				return
			}
			fmt.Printf("\n%sExisting index has insufficient points, regenerating...%s\n", colorYellow, colorReset)
		}
	}
	
	printSubtitle("Loading Points")
	
	numPoints := 1000000
	numWorkers := runtime.NumCPU()
	
	fmt.Printf("Loading %s%d%s random points using %s%d%s workers...\n", 
		colorBold, numPoints, colorReset, colorBold, numWorkers, colorReset)
	
	// Generate points
	points := generateRandomPoints(numPoints)
	
	// Create index
	index := rtree.NewGeoIndex()
	
	start := time.Now()
	
	// Load points with progress
	batchSize := numPoints / numWorkers
	if batchSize < 1 {
		batchSize = 1
	}
	
	var wg sync.WaitGroup
	var loaded atomic.Int32
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				printProgress(numPoints, numPoints, "Loading")
				return
			case <-ticker.C:
				current := int(loaded.Load())
				printProgress(current, numPoints, "Loading")
			}
		}
	}()
	
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		startIdx := i * batchSize
		endIdx := startIdx + batchSize
		if i == numWorkers-1 {
			endIdx = numPoints
		}
		
		go func(batch []*models.Point) {
			defer wg.Done()
			err := index.IndexPoints(batch)
			if err != nil {
				log.Printf("Error indexing batch: %v", err)
			}
			loaded.Add(int32(len(batch)))
		}(points[startIdx:endIdx])
	}
	
	wg.Wait()
	done <- true
	loadTime := time.Since(start)
	
	// Save index
	if err := index.SaveToFile(indexFile); err != nil {
		log.Printf("Error saving index: %v", err)
	}
	
	fmt.Println()
	printSuccess(fmt.Sprintf("Loaded %d points in %v", numPoints, loadTime))
	printSuccess(fmt.Sprintf("Points per second: %.0f", float64(numPoints)/loadTime.Seconds()))
	printSuccess(fmt.Sprintf("Index saved to %s", indexFile))
}

type benchmarkStats struct {
	queriesPerSecond float64
	avgQueryTime     time.Duration
	totalQueries     int64
}

func runBenchmarks() benchmarkStats {
	printSubtitle("Running Bounding Box Queries")
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	benchDuration := 10 * time.Second
	numWorkers := runtime.NumCPU()
	
	fmt.Printf("Running bounding box queries for %s%v%s using %s%d%s workers...\n",
		colorBold, benchDuration, colorReset, colorBold, numWorkers, colorReset)
	
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
		}()
	}
	
	wg.Wait()
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("R-Tree Bounding Box Queries Complete!")
	printStat("Queries per second", fmt.Sprintf("%s%.0f%s", colorGreen, float64(completedQueries)/elapsed.Seconds(), colorReset))
	printStat("Average query time", fmt.Sprintf("%s%v%s", colorGreen, elapsed/time.Duration(completedQueries), colorReset))
	
	return benchmarkStats{
		queriesPerSecond: float64(completedQueries)/elapsed.Seconds(),
		avgQueryTime:     elapsed/time.Duration(completedQueries),
		totalQueries:     completedQueries,
	}
}

func runPostGISBenchmark() benchmarkStats {
	printSubtitle("Running PostGIS Bounding Box Queries")
	
	// Connect to PostGIS
	db, err := postgis.NewPostGISIndex("localhost", "geouser", "geopass", "geodb", 5432)
	if err != nil {
		log.Printf("Failed to connect to PostGIS: %v", err)
		printError("PostGIS connection failed - is Docker running?")
		return benchmarkStats{}
	}
	defer db.Close()
	
	// Check if data is already loaded
	count, err := db.Count()
	if err == nil && count >= 1000000 {
		printInfo(fmt.Sprintf("Found %d points in PostGIS database", count))
	} else {
		// Load data into PostGIS
		printInfo("Loading points into PostGIS...")
		
		// Initialize schema
		if err := db.InitSchema(); err != nil {
			log.Printf("Failed to initialize schema: %v", err)
			return benchmarkStats{}
		}
		
		// Generate same points
		points := generateRandomPoints(1000000)
		
		// Bulk insert
		start := time.Now()
		if err := db.BulkInsertPoints(points); err != nil {
			log.Printf("Failed to insert points: %v", err)
			return benchmarkStats{}
		}
		
		// Create spatial index
		if err := db.CreateSpatialIndex(); err != nil {
			log.Printf("Failed to create spatial index: %v", err)
			return benchmarkStats{}
		}
		
		elapsed := time.Since(start)
		printSuccess(fmt.Sprintf("Loaded %d points into PostGIS in %v", len(points), elapsed))
	}
	
	benchDuration := 10 * time.Second
	numWorkers := runtime.NumCPU()
	
	fmt.Printf("Running PostGIS bounding box queries for %s%v%s using %s%d%s workers...\n",
		colorBold, benchDuration, colorReset, colorBold, numWorkers, colorReset)
	
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
				}
			}
		}()
	}
	
	wg.Wait()
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("PostGIS Bounding Box Queries Complete!")
	printStat("Queries per second", fmt.Sprintf("%s%.0f%s", colorGreen, float64(completedQueries)/elapsed.Seconds(), colorReset))
	printStat("Average query time", fmt.Sprintf("%s%v%s", colorGreen, elapsed/time.Duration(completedQueries), colorReset))
	
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
	
	fmt.Printf("\n%s%-20s %s %s%s\n", colorBold, "Metric", "R-Tree", "PostGIS", colorReset)
	fmt.Println(strings.Repeat("-", 50))
	
	// Queries per second
	rtreeQPS := fmt.Sprintf("%.0f", rtreeStats.queriesPerSecond)
	postgisQPS := fmt.Sprintf("%.0f", postgisStats.queriesPerSecond)
	fmt.Printf("%-20s %s%-15s%s %s%-15s%s\n", "Queries/second", 
		colorGreen, rtreeQPS, colorReset,
		colorYellow, postgisQPS, colorReset)
	
	// Average query time
	rtreeAvg := rtreeStats.avgQueryTime.String()
	postgisAvg := postgisStats.avgQueryTime.String()
	fmt.Printf("%-20s %s%-15s%s %s%-15s%s\n", "Avg query time",
		colorGreen, rtreeAvg, colorReset,
		colorYellow, postgisAvg, colorReset)
	
	// Total queries
	fmt.Printf("%-20s %-15d %-15d\n", "Total queries", 
		rtreeStats.totalQueries, postgisStats.totalQueries)
	
	// Performance ratio
	if postgisStats.queriesPerSecond > 0 {
		ratio := rtreeStats.queriesPerSecond / postgisStats.queriesPerSecond
		fmt.Printf("\n%sR-Tree is %.1fx faster than PostGIS%s\n", colorBold, ratio, colorReset)
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
	printInfo(fmt.Sprintf("Parallel loading using %d CPU cores", runtime.NumCPU()))
	printInfo("Efficient bounding box queries")
	// printInfo("Fast radius searches (50km)")
	// printInfo("Quick k-nearest neighbor lookups (k=10)")
	
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