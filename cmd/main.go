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

	"github.com/1F47E/geo-index-rtree/pkg/geo"
	"github.com/spf13/cobra"
)

var (
	indexFile string
	verbose   bool
)

var rootCmd = &cobra.Command{
	Use:   "go-geo-index",
	Short: "High-performance R-Tree based geographical indexing demo",
	Long:  `A demonstration of R-Tree technology for efficient geo-spatial queries using Go's concurrency features.`,
}

var loadCmd = &cobra.Command{
	Use:   "load",
	Short: "Load random points into the index",
	Long:  `Generate and load random geographical points into the R-Tree index.`,
	Run:   runLoad,
}

var queryCmd = &cobra.Command{
	Use:   "query",
	Short: "Run benchmark queries on the index",
	Long:  `Execute benchmark queries (bounding box searches) on the loaded index.`,
	Run:   runQuery,
}

var radiusCmd = &cobra.Command{
	Use:   "radius",
	Short: "Run radius search benchmarks",
	Long:  `Execute radius search benchmarks on the loaded index.`,
	Run:   runRadius,
}

var nearestCmd = &cobra.Command{
	Use:   "nearest",
	Short: "Run nearest neighbor benchmarks",
	Long:  `Execute nearest neighbor search benchmarks on the loaded index.`,
	Run:   runNearest,
}

var (
	numPoints     int
	numQueries    int
	numNeighbors  int
	searchRadius  float64
	numWorkers    int
)

func init() {
	rootCmd.PersistentFlags().StringVarP(&indexFile, "file", "f", "geo_index.gob", "Index file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	loadCmd.Flags().IntVarP(&numPoints, "points", "p", 1000000, "Number of points to generate")
	loadCmd.Flags().IntVarP(&numWorkers, "workers", "w", runtime.NumCPU(), "Number of worker goroutines")

	queryCmd.Flags().IntVarP(&numQueries, "queries", "q", 1000, "Number of queries to run")
	queryCmd.Flags().IntVarP(&numWorkers, "workers", "w", runtime.NumCPU(), "Number of worker goroutines")

	radiusCmd.Flags().IntVarP(&numQueries, "queries", "q", 1000, "Number of queries to run")
	radiusCmd.Flags().Float64VarP(&searchRadius, "radius", "r", 50.0, "Search radius in km")
	radiusCmd.Flags().IntVarP(&numWorkers, "workers", "w", runtime.NumCPU(), "Number of worker goroutines")

	nearestCmd.Flags().IntVarP(&numQueries, "queries", "q", 1000, "Number of queries to run")
	nearestCmd.Flags().IntVarP(&numNeighbors, "neighbors", "n", 10, "Number of nearest neighbors to find")
	nearestCmd.Flags().IntVarP(&numWorkers, "workers", "w", runtime.NumCPU(), "Number of worker goroutines")

	rootCmd.AddCommand(loadCmd, queryCmd, radiusCmd, nearestCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runLoad(cmd *cobra.Command, args []string) {
	fmt.Printf("Loading %d random points into R-Tree index using %d workers...\n", numPoints, numWorkers)
	
	// Generate random points
	points := generateRandomPoints(numPoints)
	
	// Create index
	index := geo.NewGeoIndex()
	
	// Measure loading time
	start := time.Now()
	
	// Split points into batches for parallel processing
	batchSize := numPoints / numWorkers
	if batchSize < 1 {
		batchSize = 1
	}
	
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		startIdx := i * batchSize
		endIdx := startIdx + batchSize
		if i == numWorkers-1 {
			endIdx = numPoints
		}
		
		go func(batch []*geo.Point) {
			defer wg.Done()
			index.IndexPoints(batch)
		}(points[startIdx:endIdx])
	}
	
	wg.Wait()
	loadTime := time.Since(start)
	
	fmt.Printf("Loaded %d points in %v\n", index.Size(), loadTime)
	fmt.Printf("Points per second: %.0f\n", float64(numPoints)/loadTime.Seconds())
	
	// Save to file
	if err := index.SaveToFile(indexFile); err != nil {
		log.Fatalf("Failed to save index: %v", err)
	}
	
	fmt.Printf("Index saved to %s\n", indexFile)
}

func runQuery(cmd *cobra.Command, args []string) {
	// Load index
	index := geo.NewGeoIndex()
	fmt.Printf("Loading index from %s...\n", indexFile)
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	fmt.Printf("Loaded %d points\n", index.Size())
	fmt.Printf("Running %d bounding box queries using %d workers...\n", numQueries, numWorkers)
	
	// Prepare random bounding boxes
	queries := make([]struct{ latBL, lonBL, latTR, lonTR float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		// Random center point
		centerLat := rand.Float64()*180 - 90
		centerLon := rand.Float64()*360 - 180
		
		// Random box size (0.1 to 2 degrees)
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
	var queryCount atomic.Int64
	
	start := time.Now()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(workerID, start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				q := queries[i]
				results, err := index.SearchBox(q.latBL, q.lonBL, q.latTR, q.lonTR)
				if err != nil {
					log.Printf("Worker %d: Query error: %v", workerID, err)
					continue
				}
				localResults += len(results)
				queryCount.Add(1)
				
				if verbose && i%100 == 0 {
					fmt.Printf("Worker %d: Query %d found %d results\n", workerID, i, len(results))
				}
			}
			totalResults.Add(int64(localResults))
		}(w, startIdx, endIdx)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Printf("\nBenchmark Results:\n")
	fmt.Printf("Total queries: %d\n", completedQueries)
	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Queries per second: %.0f\n", float64(completedQueries)/elapsed.Seconds())
	fmt.Printf("Average query time: %v\n", elapsed/time.Duration(completedQueries))
	fmt.Printf("Total results found: %d\n", totalResults.Load())
	fmt.Printf("Average results per query: %.1f\n", float64(totalResults.Load())/float64(completedQueries))
}

func runRadius(cmd *cobra.Command, args []string) {
	// Load index
	index := geo.NewGeoIndex()
	fmt.Printf("Loading index from %s...\n", indexFile)
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	fmt.Printf("Loaded %d points\n", index.Size())
	fmt.Printf("Running %d radius searches (%.1f km) using %d workers...\n", numQueries, searchRadius, numWorkers)
	
	// Prepare random center points
	centers := make([]struct{ lat, lon float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		centers[i] = struct{ lat, lon float64 }{
			lat: rand.Float64()*180 - 90,
			lon: rand.Float64()*360 - 180,
		}
	}
	
	// Run benchmark
	var totalResults atomic.Int64
	var queryCount atomic.Int64
	
	start := time.Now()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(workerID, start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				c := centers[i]
				results, err := index.SearchRadius(c.lat, c.lon, searchRadius)
				if err != nil {
					log.Printf("Worker %d: Query error: %v", workerID, err)
					continue
				}
				localResults += len(results)
				queryCount.Add(1)
				
				if verbose && i%100 == 0 {
					fmt.Printf("Worker %d: Query %d found %d results\n", workerID, i, len(results))
				}
			}
			totalResults.Add(int64(localResults))
		}(w, startIdx, endIdx)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Printf("\nRadius Search Benchmark Results:\n")
	fmt.Printf("Total queries: %d\n", completedQueries)
	fmt.Printf("Search radius: %.1f km\n", searchRadius)
	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Queries per second: %.0f\n", float64(completedQueries)/elapsed.Seconds())
	fmt.Printf("Average query time: %v\n", elapsed/time.Duration(completedQueries))
	fmt.Printf("Total results found: %d\n", totalResults.Load())
	fmt.Printf("Average results per query: %.1f\n", float64(totalResults.Load())/float64(completedQueries))
}

func runNearest(cmd *cobra.Command, args []string) {
	// Load index
	index := geo.NewGeoIndex()
	fmt.Printf("Loading index from %s...\n", indexFile)
	if err := index.LoadFromFile(indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	
	fmt.Printf("Loaded %d points\n", index.Size())
	fmt.Printf("Running %d nearest neighbor searches (k=%d) using %d workers...\n", numQueries, numNeighbors, numWorkers)
	
	// Prepare random query points
	queryPoints := make([]struct{ lat, lon float64 }, numQueries)
	for i := 0; i < numQueries; i++ {
		queryPoints[i] = struct{ lat, lon float64 }{
			lat: rand.Float64()*180 - 90,
			lon: rand.Float64()*360 - 180,
		}
	}
	
	// Run benchmark
	var totalResults atomic.Int64
	var queryCount atomic.Int64
	
	start := time.Now()
	
	var wg sync.WaitGroup
	queriesPerWorker := numQueries / numWorkers
	
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startIdx := w * queriesPerWorker
		endIdx := startIdx + queriesPerWorker
		if w == numWorkers-1 {
			endIdx = numQueries
		}
		
		go func(workerID, start, end int) {
			defer wg.Done()
			
			localResults := 0
			for i := start; i < end; i++ {
				q := queryPoints[i]
				results := index.NearestNeighbors(q.lat, q.lon, numNeighbors)
				localResults += len(results)
				queryCount.Add(1)
				
				if verbose && i%100 == 0 {
					fmt.Printf("Worker %d: Query %d found %d neighbors\n", workerID, i, len(results))
				}
			}
			totalResults.Add(int64(localResults))
		}(w, startIdx, endIdx)
	}
	
	wg.Wait()
	elapsed := time.Since(start)
	
	completedQueries := queryCount.Load()
	fmt.Printf("\nNearest Neighbor Benchmark Results:\n")
	fmt.Printf("Total queries: %d\n", completedQueries)
	fmt.Printf("Neighbors requested: %d\n", numNeighbors)
	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Queries per second: %.0f\n", float64(completedQueries)/elapsed.Seconds())
	fmt.Printf("Average query time: %v\n", elapsed/time.Duration(completedQueries))
	fmt.Printf("Total results found: %d\n", totalResults.Load())
}

func generateRandomPoints(n int) []*geo.Point {
	points := make([]*geo.Point, n)
	
	// Use multiple goroutines to generate points in parallel
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
				// Generate more realistic distribution of points
				// Concentrate around major population centers
				var lat, lon float64
				
				switch r.Intn(5) {
				case 0: // North America
					lat = r.Float64()*30 + 30    // 30-60
					lon = r.Float64()*60 - 120   // -120 to -60
				case 1: // Europe
					lat = r.Float64()*20 + 40    // 40-60
					lon = r.Float64()*40 - 10    // -10 to 30
				case 2: // Asia
					lat = r.Float64()*40 + 20    // 20-60
					lon = r.Float64()*80 + 60    // 60 to 140
				case 3: // South America
					lat = r.Float64()*40 - 50    // -50 to -10
					lon = r.Float64()*30 - 80    // -80 to -50
				default: // Random
					lat = r.Float64()*180 - 90   // -90 to 90
					lon = r.Float64()*360 - 180  // -180 to 180
				}
				
				points[i] = &geo.Point{
					ID:  fmt.Sprintf("point_%d", i),
					Lat: lat,
					Lon: lon,
				}
			}
		}(startIdx, endIdx)
	}
	
	wg.Wait()
	return points
}