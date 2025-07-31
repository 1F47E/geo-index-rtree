package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"runtime"
	"time"

	"github.com/1F47E/geo-index-rtree/pkg/models"
	"github.com/1F47E/geo-index-rtree/pkg/rtree"
)

func main() {
	var (
		numPoints  = flag.Int("n", 1000000, "Number of points to generate")
		outputFile = flag.String("o", "data/index.gob", "Output file path")
		workers    = flag.Int("w", runtime.NumCPU(), "Number of worker goroutines")
		seed       = flag.Int64("seed", time.Now().UnixNano(), "Random seed")
		// Geographic bounds for random point generation (default: roughly USA)
		minLat = flag.Float64("min-lat", 25.0, "Minimum latitude")
		maxLat = flag.Float64("max-lat", 49.0, "Maximum latitude")
		minLon = flag.Float64("min-lon", -125.0, "Minimum longitude")
		maxLon = flag.Float64("max-lon", -66.0, "Maximum longitude")
	)
	flag.Parse()

	// Ensure output directory exists
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	log.Printf("Generating %d random points with %d workers...\n", *numPoints, *workers)
	log.Printf("Geographic bounds: lat[%.2f, %.2f], lon[%.2f, %.2f]\n", 
		*minLat, *maxLat, *minLon, *maxLon)

	// Initialize random generator
	rand.Seed(*seed)

	// Generate points in parallel
	points := generateRandomPoints(*numPoints, *minLat, *maxLat, *minLon, *maxLon, *workers)

	// Create index
	log.Println("Building R-Tree index...")
	startTime := time.Now()
	
	index := rtree.NewGeoIndexWithWorkers(*workers)
	if err := index.IndexPoints(points); err != nil {
		log.Fatalf("Failed to index points: %v", err)
	}
	
	indexTime := time.Since(startTime)
	log.Printf("Index built in %v (%.2f points/sec)\n", 
		indexTime, float64(*numPoints)/indexTime.Seconds())

	// Save to file
	log.Printf("Saving index to %s...\n", *outputFile)
	startTime = time.Now()
	
	if err := index.SaveToFile(*outputFile); err != nil {
		log.Fatalf("Failed to save index: %v", err)
	}
	
	saveTime := time.Since(startTime)
	log.Printf("Index saved in %v\n", saveTime)

	// Print statistics
	fileInfo, err := os.Stat(*outputFile)
	if err == nil {
		log.Printf("Index file size: %.2f MB\n", float64(fileInfo.Size())/(1024*1024))
	}
	log.Printf("Total points indexed: %d\n", index.Count())
}

func generateRandomPoints(n int, minLat, maxLat, minLon, maxLon float64, workers int) []*models.Point {
	points := make([]*models.Point, n)
	
	// Calculate points per worker
	pointsPerWorker := n / workers
	remainder := n % workers
	
	// Channel to coordinate work
	type workRange struct {
		start, end int
	}
	work := make(chan workRange, workers)
	done := make(chan bool, workers)
	
	// Start workers
	for w := 0; w < workers; w++ {
		go func(workerID int) {
			// Each worker gets its own random generator to avoid contention
			r := rand.New(rand.NewSource(rand.Int63()))
			
			for wr := range work {
				for i := wr.start; i < wr.end; i++ {
					lat := minLat + r.Float64()*(maxLat-minLat)
					lon := minLon + r.Float64()*(maxLon-minLon)
					
					points[i] = &models.Point{
						ID: fmt.Sprintf("point_%d", i),
						Location: &models.Location{
							Lat: lat,
							Lon: lon,
						},
					}
				}
			}
			done <- true
		}(w)
	}
	
	// Distribute work
	start := 0
	for w := 0; w < workers; w++ {
		size := pointsPerWorker
		if w < remainder {
			size++
		}
		work <- workRange{start: start, end: start + size}
		start += size
	}
	close(work)
	
	// Wait for completion
	for w := 0; w < workers; w++ {
		<-done
	}
	
	return points
}