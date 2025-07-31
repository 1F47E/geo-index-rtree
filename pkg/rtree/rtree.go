// Package rtree implements a high-performance R-Tree for geo-spatial indexing
// with goroutine-based parallel processing for maximum efficiency
package rtree

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/dhconnelly/rtreego"
	"github.com/kass/go-geo-index/pkg/models"
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
	tree      *rtreego.Rtree
	mu        sync.RWMutex
	itemCount atomic.Int64
	
	// Parallel processing configuration
	numWorkers int
}

// NewGeoIndex creates a new geographic index with optimal worker count
func NewGeoIndex() *GeoIndex {
	numWorkers := runtime.NumCPU()
	return &GeoIndex{
		tree:       rtreego.NewTree(dimensions, minChildren, maxChildren),
		numWorkers: numWorkers,
	}
}

// NewGeoIndexWithWorkers creates a new geographic index with specified worker count
func NewGeoIndexWithWorkers(workers int) *GeoIndex {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &GeoIndex{
		tree:       rtreego.NewTree(dimensions, minChildren, maxChildren),
		numWorkers: workers,
	}
}

// IndexPoints indexes multiple points in parallel using goroutines
func (g *GeoIndex) IndexPoints(points []*models.Point) error {
	if len(points) == 0 {
		return nil
	}

	// Create spatial items in parallel
	spatialItems := make([]*spatialPoint, len(points))
	
	// Use worker pool pattern for efficient goroutine usage
	workerCh := make(chan int, len(points))
	var wg sync.WaitGroup
	
	// Start worker goroutines
	wg.Add(g.numWorkers)
	for w := 0; w < g.numWorkers; w++ {
		go func() {
			defer wg.Done()
			for idx := range workerCh {
				point := points[idx]
				if point.Location == nil {
					continue
				}
				
				p := rtreego.Point{
					point.Location.Lat,
					point.Location.Lon,
				}
				rect := p.ToRect(tolerance)
				spatialItems[idx] = &spatialPoint{point, rect}
			}
		}()
	}
	
	// Send work to workers
	for i := range points {
		workerCh <- i
	}
	close(workerCh)
	
	// Wait for all workers to complete
	wg.Wait()
	
	// Insert items into tree (this needs to be sequential)
	g.mu.Lock()
	defer g.mu.Unlock()
	
	inserted := 0
	for _, item := range spatialItems {
		if item != nil {
			g.tree.Insert(item)
			inserted++
		}
	}
	
	g.itemCount.Store(int64(inserted))
	return nil
}

// QueryBox returns all points within the given bounding box
func (g *GeoIndex) QueryBox(box models.BoundingBox) ([]*models.Point, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	// Calculate bounding box dimensions
	bottomLeft := rtreego.Point{box.BottomLeft.Lat, box.BottomLeft.Lon}
	rectSize := []float64{
		box.TopRight.Lat - box.BottomLeft.Lat,
		box.TopRight.Lon - box.BottomLeft.Lon,
	}
	
	bounds, err := rtreego.NewRect(bottomLeft, rectSize)
	if err != nil {
		return nil, fmt.Errorf("invalid bounding box: %v", err)
	}
	
	results := g.tree.SearchIntersect(bounds)
	
	// Filter results to ensure they're strictly within bounds
	points := make([]*models.Point, 0, len(results))
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
	
	return points, nil
}

// QueryRadius returns all points within the given radius (in km) from a center point
func (g *GeoIndex) QueryRadius(center models.Location, radiusKm float64) ([]*models.Point, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	// Convert radius to degrees (approximate)
	deg := (radiusKm / earthRadius) * (180 / math.Pi)
	
	// Create bounding box for initial filtering
	bounds, err := rtreego.NewRect(
		rtreego.Point{center.Lat - deg, center.Lon - deg},
		[]float64{2 * deg, 2 * deg},
	)
	if err != nil {
		return nil, err
	}
	
	results := g.tree.SearchIntersect(bounds)
	
	// Filter by actual distance
	points := make([]*models.Point, 0, len(results))
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
	
	return points, nil
}

// NearestNeighbors returns the N nearest points to the given location
func (g *GeoIndex) NearestNeighbors(center models.Location, n int) []*models.Point {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	queryPoint := rtreego.Point{center.Lat, center.Lon}
	results := g.tree.NearestNeighbors(n, queryPoint)
	
	points := make([]*models.Point, len(results))
	for i, result := range results {
		points[i] = result.(*spatialPoint).Point
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
	
	g.tree = rtreego.NewTree(dimensions, minChildren, maxChildren)
	g.itemCount.Store(0)
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