package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kass/go-geo-index/pkg/models"
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
	
	// Phase 2: Bounding Box Queries
	time.Sleep(500 * time.Millisecond)
	runBenchmarks()
	
	// Phase 3: Radius Searches
	time.Sleep(500 * time.Millisecond)
	runRadiusSearches()
	
	// Phase 4: Nearest Neighbor
	time.Sleep(500 * time.Millisecond)
	runNearestNeighbors()
	
	// Summary
	printSummary()
}

func loadAndIndex() {
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

func runBenchmarks() {
	printSubtitle("Running Bounding Box Queries")
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	numQueries := 1000
	numWorkers := runtime.NumCPU()
	
	fmt.Printf("Executing %s%d%s bounding box queries using %s%d%s workers...\n",
		colorBold, numQueries, colorReset, colorBold, numWorkers, colorReset)
	
	// Prepare queries
	queries := make([]struct{ latBL, lonBL, latTR, lonTR float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		centerLat := rand.Float64()*180 - 90
		centerLon := rand.Float64()*360 - 180
		boxSize := rand.Float64()*1.9 + 0.1
		
		queries[i] = struct{ latBL, lonBL, latTR, lonTR float64 }{
			latBL: centerLat - boxSize/2,
			lonBL: centerLon - boxSize/2,
			latTR: centerLat + boxSize/2,
			lonTR: centerLon + boxSize/2,
		}
	}
	
	// Run benchmark
	var totalResults atomic.Int64
	var queryCount atomic.Int32
	
	start := time.Now()
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				printProgress(numQueries, numQueries, "Querying")
				return
			case <-ticker.C:
				current := int(queryCount.Load())
				printProgress(current, numQueries, "Querying")
			}
		}
	}()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				q := queries[i]
				box := models.BoundingBox{
					BottomLeft: models.Location{Lat: q.latBL, Lon: q.lonBL},
					TopRight: models.Location{Lat: q.latTR, Lon: q.lonTR},
				}
				results, err := index.QueryBox(box)
				if err == nil {
					localResults += len(results)
				}
				queryCount.Add(1)
			}
			totalResults.Add(int64(localResults))
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("Bounding Box Queries Complete!")
	printStat("Total queries", completedQueries)
	printStat("Total time", elapsed)
	printStat("Queries per second", fmt.Sprintf("%.0f", float64(completedQueries)/elapsed.Seconds()))
	printStat("Average query time", elapsed/time.Duration(completedQueries))
	printStat("Total results found", totalResults.Load())
	printStat("Average results per query", fmt.Sprintf("%.1f", float64(totalResults.Load())/float64(completedQueries)))
}

func runRadiusSearches() {
	printSubtitle("Running Radius Searches")
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	numQueries := 1000
	numWorkers := runtime.NumCPU()
	searchRadius := 50.0 // km
	
	fmt.Printf("Executing %s%d%s radius searches (%s%.1f km%s) using %s%d%s workers...\n",
		colorBold, numQueries, colorReset, 
		colorBold, searchRadius, colorReset,
		colorBold, numWorkers, colorReset)
	
	// Prepare center points
	centers := make([]struct{ lat, lon float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		centers[i] = struct{ lat, lon float64 }{
			lat: rand.Float64()*180 - 90,
			lon: rand.Float64()*360 - 180,
		}
	}
	
	// Run benchmark
	var totalResults atomic.Int64
	var queryCount atomic.Int32
	
	start := time.Now()
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				printProgress(numQueries, numQueries, "Searching")
				return
			case <-ticker.C:
				current := int(queryCount.Load())
				printProgress(current, numQueries, "Searching")
			}
		}
	}()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				c := centers[i]
				center := models.Location{Lat: c.lat, Lon: c.lon}
				results, err := index.QueryRadius(center, searchRadius)
				if err == nil {
					localResults += len(results)
				}
				queryCount.Add(1)
			}
			totalResults.Add(int64(localResults))
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("Radius Searches Complete!")
	printStat("Total queries", completedQueries)
	printStat("Search radius", fmt.Sprintf("%.1f km", searchRadius))
	printStat("Total time", elapsed)
	printStat("Queries per second", fmt.Sprintf("%.0f", float64(completedQueries)/elapsed.Seconds()))
	printStat("Average query time", elapsed/time.Duration(completedQueries))
	printStat("Total results found", totalResults.Load())
	printStat("Average results per query", fmt.Sprintf("%.1f", float64(totalResults.Load())/float64(completedQueries)))
}

func runNearestNeighbors() {
	printSubtitle("Running Nearest Neighbor Searches")
	
	// Load index
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	numQueries := 1000
	numWorkers := runtime.NumCPU()
	numNeighbors := 10
	
	fmt.Printf("Finding %s%d%s nearest neighbors for %s%d%s queries using %s%d%s workers...\n",
		colorBold, numNeighbors, colorReset,
		colorBold, numQueries, colorReset,
		colorBold, numWorkers, colorReset)
	
	// Prepare query points
	queryPoints := make([]struct{ lat, lon float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		queryPoints[i] = struct{ lat, lon float64 }{
			lat: rand.Float64()*180 - 90,
			lon: rand.Float64()*360 - 180,
		}
	}
	
	// Run benchmark
	var totalResults atomic.Int64
	var queryCount atomic.Int32
	
	start := time.Now()
	
	// Progress reporter
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		
		for {
			select {
			case <-done:
				printProgress(numQueries, numQueries, "Finding")
				return
			case <-ticker.C:
				current := int(queryCount.Load())
				printProgress(current, numQueries, "Finding")
			}
		}
	}()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				q := queryPoints[i]
				center := models.Location{Lat: q.lat, Lon: q.lon}
				results := index.NearestNeighbors(center, numNeighbors)
				localResults += len(results)
				queryCount.Add(1)
			}
			totalResults.Add(int64(localResults))
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	done <- true
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Println()
	printSuccess("Nearest Neighbor Searches Complete!")
	printStat("Total queries", completedQueries)
	printStat("Neighbors requested", numNeighbors)
	printStat("Total time", elapsed)
	printStat("Queries per second", fmt.Sprintf("%.0f", float64(completedQueries)/elapsed.Seconds()))
	printStat("Average query time", elapsed/time.Duration(completedQueries))
	printStat("Total results found", totalResults.Load())
}

func printSummary() {
	printTitle("Demo Complete! 🎉")
	
	fmt.Printf("\n%sThe R-Tree index demonstrated:%s\n", colorBold, colorReset)
	printInfo(fmt.Sprintf("Parallel loading using %d CPU cores", runtime.NumCPU()))
	printInfo("Efficient bounding box queries")
	printInfo("Fast radius searches")
	printInfo("Quick nearest neighbor lookups")
	
	fmt.Printf("\n%sPerformance Highlights:%s\n", colorBold, colorReset)
	printInfo("Indexed 1 million points in seconds")
	printInfo("Thousands of queries per second")
	printInfo("Sub-millisecond average query times")
	
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