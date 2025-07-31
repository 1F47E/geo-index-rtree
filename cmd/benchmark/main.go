package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kass/go-geo-index/pkg/models"
	"github.com/kass/go-geo-index/pkg/rtree"
)

type BenchmarkResult struct {
	QueryType      string
	TotalQueries   int
	TotalDuration  time.Duration
	AvgDuration    time.Duration
	QueriesPerSec  float64
	MinDuration    time.Duration
	MaxDuration    time.Duration
	TotalResults   int64
	AvgResults     float64
}

func main() {
	var (
		indexFile = flag.String("i", "data/index.gob", "Index file path")
		queryType = flag.String("t", "box", "Query type: box, radius, nearest, mixed")
		numQueries = flag.Int("n", 1000, "Number of queries to run")
		workers = flag.Int("w", runtime.NumCPU(), "Number of concurrent workers")
		// Geographic bounds for random queries (default: roughly USA)
		minLat = flag.Float64("min-lat", 25.0, "Minimum latitude for random queries")
		maxLat = flag.Float64("max-lat", 49.0, "Maximum latitude for random queries")
		minLon = flag.Float64("min-lon", -125.0, "Minimum longitude for random queries")
		maxLon = flag.Float64("max-lon", -66.0, "Maximum longitude for random queries")
		// Query-specific parameters
		boxSize = flag.Float64("box-size", 1.0, "Box size in degrees (for box queries)")
		radius = flag.Float64("radius", 50.0, "Radius in km (for radius queries)")
		k = flag.Int("k", 100, "Number of nearest neighbors")
	)
	flag.Parse()

	// Load index
	log.Printf("Loading index from %s...\n", *indexFile)
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(*indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	log.Printf("Index loaded with %d points\n", index.Count())

	// Run benchmark
	log.Printf("Running %d %s queries with %d workers...\n", *numQueries, *queryType, *workers)
	
	var result BenchmarkResult
	switch *queryType {
	case "box":
		result = benchmarkBoxQueries(index, *numQueries, *workers, 
			*minLat, *maxLat, *minLon, *maxLon, *boxSize)
	case "radius":
		result = benchmarkRadiusQueries(index, *numQueries, *workers,
			*minLat, *maxLat, *minLon, *maxLon, *radius)
	case "nearest":
		result = benchmarkNearestQueries(index, *numQueries, *workers,
			*minLat, *maxLat, *minLon, *maxLon, *k)
	case "mixed":
		result = benchmarkMixedQueries(index, *numQueries, *workers,
			*minLat, *maxLat, *minLon, *maxLon, *boxSize, *radius, *k)
	default:
		log.Fatalf("Unknown query type: %s", *queryType)
	}

	// Print results
	fmt.Println("\n=== Benchmark Results ===")
	fmt.Printf("Query Type: %s\n", result.QueryType)
	fmt.Printf("Total Queries: %d\n", result.TotalQueries)
	fmt.Printf("Total Duration: %v\n", result.TotalDuration)
	fmt.Printf("Average Duration: %v\n", result.AvgDuration)
	fmt.Printf("Queries/Second: %.2f\n", result.QueriesPerSec)
	fmt.Printf("Min Duration: %v\n", result.MinDuration)
	fmt.Printf("Max Duration: %v\n", result.MaxDuration)
	fmt.Printf("Total Results: %d\n", result.TotalResults)
	fmt.Printf("Avg Results/Query: %.2f\n", result.AvgResults)
	fmt.Printf("Workers Used: %d\n", *workers)
	fmt.Printf("CPU Cores: %d\n", runtime.NumCPU())
}

func benchmarkBoxQueries(index *rtree.GeoIndex, numQueries, workers int,
	minLat, maxLat, minLon, maxLon, boxSize float64) BenchmarkResult {
	
	var (
		totalResults int64
		minDuration = time.Hour
		maxDuration time.Duration
		durations []time.Duration
		mu sync.Mutex
	)

	startTime := time.Now()
	
	// Worker pool
	queryCh := make(chan int, numQueries)
	var wg sync.WaitGroup
	
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewSource(rand.Int63()))
			
			for range queryCh {
				// Generate random box
				lat := minLat + r.Float64()*(maxLat-minLat-boxSize)
				lon := minLon + r.Float64()*(maxLon-minLon-boxSize)
				
				box := models.BoundingBox{
					BottomLeft: models.Location{Lat: lat, Lon: lon},
					TopRight:   models.Location{Lat: lat + boxSize, Lon: lon + boxSize},
				}
				
				queryStart := time.Now()
				results, err := index.QueryBox(box)
				queryDuration := time.Since(queryStart)
				
				if err == nil {
					atomic.AddInt64(&totalResults, int64(len(results)))
					
					mu.Lock()
					durations = append(durations, queryDuration)
					if queryDuration < minDuration {
						minDuration = queryDuration
					}
					if queryDuration > maxDuration {
						maxDuration = queryDuration
					}
					mu.Unlock()
				}
			}
		}()
	}
	
	// Send queries
	for i := 0; i < numQueries; i++ {
		queryCh <- i
	}
	close(queryCh)
	
	wg.Wait()
	totalDuration := time.Since(startTime)
	
	// Calculate average duration
	var totalDur time.Duration
	for _, d := range durations {
		totalDur += d
	}
	avgDuration := totalDur / time.Duration(len(durations))
	
	return BenchmarkResult{
		QueryType:     "box",
		TotalQueries:  numQueries,
		TotalDuration: totalDuration,
		AvgDuration:   avgDuration,
		QueriesPerSec: float64(numQueries) / totalDuration.Seconds(),
		MinDuration:   minDuration,
		MaxDuration:   maxDuration,
		TotalResults:  totalResults,
		AvgResults:    float64(totalResults) / float64(numQueries),
	}
}

func benchmarkRadiusQueries(index *rtree.GeoIndex, numQueries, workers int,
	minLat, maxLat, minLon, maxLon, radius float64) BenchmarkResult {
	
	var (
		totalResults int64
		minDuration = time.Hour
		maxDuration time.Duration
		durations []time.Duration
		mu sync.Mutex
	)

	startTime := time.Now()
	
	queryCh := make(chan int, numQueries)
	var wg sync.WaitGroup
	
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewSource(rand.Int63()))
			
			for range queryCh {
				// Generate random center
				center := models.Location{
					Lat: minLat + r.Float64()*(maxLat-minLat),
					Lon: minLon + r.Float64()*(maxLon-minLon),
				}
				
				queryStart := time.Now()
				results, err := index.QueryRadius(center, radius)
				queryDuration := time.Since(queryStart)
				
				if err == nil {
					atomic.AddInt64(&totalResults, int64(len(results)))
					
					mu.Lock()
					durations = append(durations, queryDuration)
					if queryDuration < minDuration {
						minDuration = queryDuration
					}
					if queryDuration > maxDuration {
						maxDuration = queryDuration
					}
					mu.Unlock()
				}
			}
		}()
	}
	
	for i := 0; i < numQueries; i++ {
		queryCh <- i
	}
	close(queryCh)
	
	wg.Wait()
	totalDuration := time.Since(startTime)
	
	var totalDur time.Duration
	for _, d := range durations {
		totalDur += d
	}
	avgDuration := totalDur / time.Duration(len(durations))
	
	return BenchmarkResult{
		QueryType:     "radius",
		TotalQueries:  numQueries,
		TotalDuration: totalDuration,
		AvgDuration:   avgDuration,
		QueriesPerSec: float64(numQueries) / totalDuration.Seconds(),
		MinDuration:   minDuration,
		MaxDuration:   maxDuration,
		TotalResults:  totalResults,
		AvgResults:    float64(totalResults) / float64(numQueries),
	}
}

func benchmarkNearestQueries(index *rtree.GeoIndex, numQueries, workers int,
	minLat, maxLat, minLon, maxLon float64, k int) BenchmarkResult {
	
	var (
		totalResults int64
		minDuration = time.Hour
		maxDuration time.Duration
		durations []time.Duration
		mu sync.Mutex
	)

	startTime := time.Now()
	
	queryCh := make(chan int, numQueries)
	var wg sync.WaitGroup
	
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			r := rand.New(rand.NewSource(rand.Int63()))
			
			for range queryCh {
				// Generate random center
				center := models.Location{
					Lat: minLat + r.Float64()*(maxLat-minLat),
					Lon: minLon + r.Float64()*(maxLon-minLon),
				}
				
				queryStart := time.Now()
				results := index.NearestNeighbors(center, k)
				queryDuration := time.Since(queryStart)
				
				atomic.AddInt64(&totalResults, int64(len(results)))
				
				mu.Lock()
				durations = append(durations, queryDuration)
				if queryDuration < minDuration {
					minDuration = queryDuration
				}
				if queryDuration > maxDuration {
					maxDuration = queryDuration
				}
				mu.Unlock()
			}
		}()
	}
	
	for i := 0; i < numQueries; i++ {
		queryCh <- i
	}
	close(queryCh)
	
	wg.Wait()
	totalDuration := time.Since(startTime)
	
	var totalDur time.Duration
	for _, d := range durations {
		totalDur += d
	}
	avgDuration := totalDur / time.Duration(len(durations))
	
	return BenchmarkResult{
		QueryType:     "nearest",
		TotalQueries:  numQueries,
		TotalDuration: totalDuration,
		AvgDuration:   avgDuration,
		QueriesPerSec: float64(numQueries) / totalDuration.Seconds(),
		MinDuration:   minDuration,
		MaxDuration:   maxDuration,
		TotalResults:  totalResults,
		AvgResults:    float64(totalResults) / float64(numQueries),
	}
}

func benchmarkMixedQueries(index *rtree.GeoIndex, numQueries, workers int,
	minLat, maxLat, minLon, maxLon, boxSize, radius float64, k int) BenchmarkResult {
	
	// Run 1/3 of each query type
	queriesPerType := numQueries / 3
	
	log.Println("Running mixed benchmark (33% each type)...")
	
	boxResult := benchmarkBoxQueries(index, queriesPerType, workers, 
		minLat, maxLat, minLon, maxLon, boxSize)
	radiusResult := benchmarkRadiusQueries(index, queriesPerType, workers,
		minLat, maxLat, minLon, maxLon, radius)
	nearestResult := benchmarkNearestQueries(index, queriesPerType, workers,
		minLat, maxLat, minLon, maxLon, k)
	
	// Combine results
	totalQueries := boxResult.TotalQueries + radiusResult.TotalQueries + nearestResult.TotalQueries
	totalDuration := boxResult.TotalDuration + radiusResult.TotalDuration + nearestResult.TotalDuration
	totalResults := boxResult.TotalResults + radiusResult.TotalResults + nearestResult.TotalResults
	
	return BenchmarkResult{
		QueryType:     "mixed",
		TotalQueries:  totalQueries,
		TotalDuration: totalDuration,
		AvgDuration:   totalDuration / time.Duration(totalQueries),
		QueriesPerSec: float64(totalQueries) / totalDuration.Seconds(),
		MinDuration:   min(boxResult.MinDuration, radiusResult.MinDuration, nearestResult.MinDuration),
		MaxDuration:   max(boxResult.MaxDuration, radiusResult.MaxDuration, nearestResult.MaxDuration),
		TotalResults:  totalResults,
		AvgResults:    float64(totalResults) / float64(totalQueries),
	}
}

func min(durations ...time.Duration) time.Duration {
	minD := durations[0]
	for _, d := range durations[1:] {
		if d < minD {
			minD = d
		}
	}
	return minD
}

func max(durations ...time.Duration) time.Duration {
	maxD := durations[0]
	for _, d := range durations[1:] {
		if d > maxD {
			maxD = d
		}
	}
	return maxD
}