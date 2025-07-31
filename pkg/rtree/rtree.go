// Package rtree implements a high-performance R-Tree for geo-spatial indexing
// with goroutine-based parallel processing for maximum efficiency
package rtree

import (
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/dhconnelly/rtreego"
	"github.com/1F47E/geo-index-rtree/pkg/models"
)

const (
	tolerance    = 0.01
	minChildren  = 25
	maxChildren  = 50
	dimensions   = 2
	earthRadius  = 6371.0 // km
)

// spatialPoint wraps a point to implement rtreego.Spatial interface
type spatialPoint struct {
	*models.Point
	rect *rtreego.Rect
}

func (sp *spatialPoint) Bounds() *rtreego.Rect {
	return sp.rect
}

// GeoIndex represents a thread-safe R-Tree based geographic index
type GeoIndex struct {
	// Partitioned trees for parallel query execution
	partitions []*rtreego.Rtree
	numCPU     int
	mu         sync.RWMutex
	itemCount  atomic.Int64
	
	// Partition bounds for efficient query routing
	partitionBounds []models.BoundingBox
}

// NewGeoIndex creates a new geographic index with CPU-aware partitioning
func NewGeoIndex() *GeoIndex {
	numCPU := runtime.NumCPU()
	partitions := make([]*rtreego.Rtree, numCPU)
	partitionBounds := make([]models.BoundingBox, numCPU)
	
	// Create partitions based on longitude bands
	lonRange := 360.0 / float64(numCPU)
	for i := 0; i < numCPU; i++ {
		partitions[i] = rtreego.NewTree(dimensions, minChildren, maxChildren)
		
		// Calculate partition bounds
		minLon := -180.0 + float64(i)*lonRange
		maxLon := minLon + lonRange
		if i == numCPU-1 {
			maxLon = 180.0 // Ensure last partition covers all remaining space
		}
		
		partitionBounds[i] = models.BoundingBox{
			BottomLeft: models.Location{Lat: -90, Lon: minLon},
			TopRight:   models.Location{Lat: 90, Lon: maxLon},
		}
	}
	
	return &GeoIndex{
		partitions:      partitions,
		numCPU:          numCPU,
		partitionBounds: partitionBounds,
	}
}

// NewGeoIndexWithWorkers creates a new geographic index with specified partition count
func NewGeoIndexWithWorkers(numPartitions int) *GeoIndex {
	if numPartitions <= 0 {
		numPartitions = runtime.NumCPU()
	}
	
	partitions := make([]*rtreego.Rtree, numPartitions)
	partitionBounds := make([]models.BoundingBox, numPartitions)
	
	// Create partitions based on longitude bands
	lonRange := 360.0 / float64(numPartitions)
	for i := 0; i < numPartitions; i++ {
		partitions[i] = rtreego.NewTree(dimensions, minChildren, maxChildren)
		
		// Calculate partition bounds
		minLon := -180.0 + float64(i)*lonRange
		maxLon := minLon + lonRange
		if i == numPartitions-1 {
			maxLon = 180.0
		}
		
		partitionBounds[i] = models.BoundingBox{
			BottomLeft: models.Location{Lat: -90, Lon: minLon},
			TopRight:   models.Location{Lat: 90, Lon: maxLon},
		}
	}
	
	return &GeoIndex{
		partitions:      partitions,
		numCPU:          numPartitions,
		partitionBounds: partitionBounds,
	}
}

// IndexPoints indexes multiple points using spatial partitioning
func (g *GeoIndex) IndexPoints(points []*models.Point) error {
	if len(points) == 0 {
		return nil
	}

	// Group points by partition
	partitionedPoints := make([][]*spatialPoint, g.numCPU)
	for i := range partitionedPoints {
		partitionedPoints[i] = make([]*spatialPoint, 0, len(points)/g.numCPU)
	}
	
	// Distribute points to partitions based on longitude
	lonRange := 360.0 / float64(g.numCPU)
	for _, point := range points {
		if point.Location == nil {
			continue
		}
		
		// Create spatial point
		p := rtreego.Point{
			point.Location.Lat,
			point.Location.Lon,
		}
		rect := p.ToRect(tolerance)
		spatialPoint := &spatialPoint{point, rect}
		
		// Determine partition based on longitude
		partitionIdx := int((point.Location.Lon + 180.0) / lonRange)
		if partitionIdx >= g.numCPU {
			partitionIdx = g.numCPU - 1
		}
		if partitionIdx < 0 {
			partitionIdx = 0
		}
		
		partitionedPoints[partitionIdx] = append(partitionedPoints[partitionIdx], spatialPoint)
	}
	
	// Insert into partitions in parallel
	g.mu.Lock()
	defer g.mu.Unlock()
	
	var wg sync.WaitGroup
	var totalInserted atomic.Int64
	
	for i := 0; i < g.numCPU; i++ {
		if len(partitionedPoints[i]) == 0 {
			continue
		}
		
		wg.Add(1)
		go func(partitionIdx int, items []*spatialPoint) {
			defer wg.Done()
			
			// Each partition can be updated independently
			for _, item := range items {
				g.partitions[partitionIdx].Insert(item)
			}
			totalInserted.Add(int64(len(items)))
		}(i, partitionedPoints[i])
	}
	
	wg.Wait()
	g.itemCount.Store(totalInserted.Load())
	return nil
}

// QueryBox returns all points within the given bounding box using parallel search
func (g *GeoIndex) QueryBox(box models.BoundingBox) ([]*models.Point, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	// Determine which partitions to search
	relevantPartitions := g.getRelevantPartitions(box)
	
	// Create channels for results
	resultsChan := make(chan []*models.Point, len(relevantPartitions))
	
	// Search partitions in parallel
	for _, partitionIdx := range relevantPartitions {
		go func(idx int) {
			// Calculate bounding box dimensions
			bottomLeft := rtreego.Point{box.BottomLeft.Lat, box.BottomLeft.Lon}
			rectSize := []float64{
				box.TopRight.Lat - box.BottomLeft.Lat,
				box.TopRight.Lon - box.BottomLeft.Lon,
			}
			
			bounds, err := rtreego.NewRect(bottomLeft, rectSize)
			if err != nil {
				resultsChan <- nil
				return
			}
			
			// Search this partition
			results := g.partitions[idx].SearchIntersect(bounds)
			
			// Filter results to ensure they're strictly within bounds
			points := make([]*models.Point, 0)
			for _, result := range results {
				item, ok := result.(*spatialPoint)
				if !ok || item.Point == nil || item.Point.Location == nil {
					continue
				}
				
				// Strict boundary check
				loc := item.Point.Location
				if loc.Lat >= box.BottomLeft.Lat && loc.Lat <= box.TopRight.Lat &&
				   loc.Lon >= box.BottomLeft.Lon && loc.Lon <= box.TopRight.Lon {
					points = append(points, item.Point)
				}
			}
			
			resultsChan <- points
		}(partitionIdx)
	}
	
	// Merge results from all partitions
	var allResults []*models.Point
	for i := 0; i < len(relevantPartitions); i++ {
		partitionResults := <-resultsChan
		if partitionResults != nil {
			allResults = append(allResults, partitionResults...)
		}
	}
	
	return allResults, nil
}

// QueryRadius returns all points within the given radius (in km) from a center point using parallel search
func (g *GeoIndex) QueryRadius(center models.Location, radiusKm float64) ([]*models.Point, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	// Convert radius to degrees (approximate)
	deg := (radiusKm / earthRadius) * (180 / math.Pi)
	
	// Create bounding box for initial filtering
	queryBox := models.BoundingBox{
		BottomLeft: models.Location{Lat: center.Lat - deg, Lon: center.Lon - deg},
		TopRight:   models.Location{Lat: center.Lat + deg, Lon: center.Lon + deg},
	}
	
	// Determine which partitions to search
	relevantPartitions := g.getRelevantPartitions(queryBox)
	
	// Create channels for results
	resultsChan := make(chan []*models.Point, len(relevantPartitions))
	
	// Search partitions in parallel
	for _, partitionIdx := range relevantPartitions {
		go func(idx int) {
			bounds, err := rtreego.NewRect(
				rtreego.Point{center.Lat - deg, center.Lon - deg},
				[]float64{2 * deg, 2 * deg},
			)
			if err != nil {
				resultsChan <- nil
				return
			}
			
			results := g.partitions[idx].SearchIntersect(bounds)
			
			// Filter by actual distance
			points := make([]*models.Point, 0)
			for _, result := range results {
				item, ok := result.(*spatialPoint)
				if !ok || item.Point == nil || item.Point.Location == nil {
					continue
				}
				
				dist := Distance(center.Lat, center.Lon, 
					item.Point.Location.Lat, item.Point.Location.Lon)
				if dist <= radiusKm {
					points = append(points, item.Point)
				}
			}
			
			resultsChan <- points
		}(partitionIdx)
	}
	
	// Merge results from all partitions
	var allResults []*models.Point
	for i := 0; i < len(relevantPartitions); i++ {
		partitionResults := <-resultsChan
		if partitionResults != nil {
			allResults = append(allResults, partitionResults...)
		}
	}
	
	return allResults, nil
}

// NearestNeighbors returns the N nearest points to the given location using parallel search
func (g *GeoIndex) NearestNeighbors(center models.Location, n int) []*models.Point {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	type nearestResult struct {
		point    *models.Point
		distance float64
	}
	
	// Search all partitions in parallel
	resultsChan := make(chan []nearestResult, g.numCPU)
	
	for i := 0; i < g.numCPU; i++ {
		go func(idx int) {
			queryPoint := rtreego.Point{center.Lat, center.Lon}
			// Get more candidates than needed from each partition
			results := g.partitions[idx].NearestNeighbors(n*2, queryPoint)
			
			nearestResults := make([]nearestResult, 0, len(results))
			for _, result := range results {
				sp := result.(*spatialPoint)
				dist := Distance(center.Lat, center.Lon,
					sp.Point.Location.Lat, sp.Point.Location.Lon)
				nearestResults = append(nearestResults, nearestResult{
					point:    sp.Point,
					distance: dist,
				})
			}
			
			resultsChan <- nearestResults
		}(i)
	}
	
	// Collect all results
	var allResults []nearestResult
	for i := 0; i < g.numCPU; i++ {
		partitionResults := <-resultsChan
		allResults = append(allResults, partitionResults...)
	}
	
	// Sort by distance and take top n
	for i := 0; i < len(allResults); i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[i].distance > allResults[j].distance {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}
	
	// Return top n points
	resultCount := n
	if len(allResults) < n {
		resultCount = len(allResults)
	}
	
	points := make([]*models.Point, resultCount)
	for i := 0; i < resultCount; i++ {
		points[i] = allResults[i].point
	}
	
	return points
}

// Count returns the number of indexed points
func (g *GeoIndex) Count() int64 {
	return g.itemCount.Load()
}

// Clear removes all points from the index
func (g *GeoIndex) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	for i := 0; i < g.numCPU; i++ {
		g.partitions[i] = rtreego.NewTree(dimensions, minChildren, maxChildren)
	}
	g.itemCount.Store(0)
}

// getRelevantPartitions returns the indices of partitions that intersect with the given bounding box
func (g *GeoIndex) getRelevantPartitions(box models.BoundingBox) []int {
	var relevant []int
	for i, bounds := range g.partitionBounds {
		// Check if partition bounds intersect with query box
		if box.BottomLeft.Lon <= bounds.TopRight.Lon &&
		   box.TopRight.Lon >= bounds.BottomLeft.Lon {
			relevant = append(relevant, i)
		}
	}
	return relevant
}

// Distance calculates the Haversine distance between two points in kilometers
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	lat1Rad := lat1 * math.Pi / 180.0
	lon1Rad := lon1 * math.Pi / 180.0
	lat2Rad := lat2 * math.Pi / 180.0
	lon2Rad := lon2 * math.Pi / 180.0
	
	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad
	
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
		math.Sin(dLon/2)*math.Sin(dLon/2)
	
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}