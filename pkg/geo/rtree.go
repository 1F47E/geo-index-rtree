// Package geo provides an efficient R-Tree implementation for geo-spatial search.
// This implementation is optimized for maximum performance using goroutines
// to parallelize operations across CPU cores.
package geo

import (
	"encoding/gob"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/dhconnelly/rtreego"
)

const (
	tolerance    = 0.01
	minChildren  = 25
	maxChildren  = 50
	dimensions   = 2
	earthRadius  = 6371.0 // km
)

// Point represents a geographical point
type Point struct {
	ID  string
	Lat float64
	Lon float64
}

// spatialItem wraps a Point for R-Tree indexing
type spatialItem struct {
	*Point
	rect *rtreego.Rect
}

func (si *spatialItem) Bounds() *rtreego.Rect {
	return si.rect
}

// GeoIndex is a thread-safe R-Tree based geographical index
type GeoIndex struct {
	tree     *rtreego.Rtree
	mu       sync.RWMutex
	itemCount atomic.Int64
}

// NewGeoIndex creates a new geographical index
func NewGeoIndex() *GeoIndex {
	return &GeoIndex{
		tree: rtreego.NewTree(dimensions, minChildren, maxChildren),
	}
}

// IndexPoints indexes a batch of points using parallel processing
func (g *GeoIndex) IndexPoints(points []*Point) {
	if len(points) == 0 {
		return
	}
	
	numCPU := runtime.NumCPU()
	spatialItems := make([]rtreego.Spatial, len(points))
	var wg sync.WaitGroup
	
	// Calculate batch size for each CPU
	batchSize := len(points) / numCPU
	if batchSize < 1 {
		batchSize = 1
		numCPU = len(points)
	}

	// Process points in parallel
	for i := 0; i < numCPU && i*batchSize < len(points); i++ {
		wg.Add(1)
		start := i * batchSize
		end := start + batchSize
		if end > len(points) {
			end = len(points)
		}
		
		go func(start, end int) {
			defer wg.Done()
			for j := start; j < end; j++ {
				point := points[j]
				if point == nil {
					continue
				}
				rtPoint := rtreego.Point{point.Lat, point.Lon}
				rect := rtPoint.ToRect(tolerance)
				spatialItems[j] = &spatialItem{point, rect}
			}
		}(start, end)
	}

	wg.Wait()

	// Insert items into the tree (this part must be synchronized)
	g.mu.Lock()
	defer g.mu.Unlock()
	
	count := int64(0)
	for _, item := range spatialItems {
		if item != nil {
			g.tree.Insert(item)
			count++
		}
	}
	g.itemCount.Add(count)
}

// SearchBox returns all points within the given bounding box
// Box is defined by bottom-left (latBL, lonBL) and top-right (latTR, lonTR) corners
func (g *GeoIndex) SearchBox(latBL, lonBL, latTR, lonTR float64) ([]*Point, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Create bounding box
	bottomLeftPoint := rtreego.Point{latBL, lonBL}
	rectSize := []float64{latTR - latBL, lonTR - lonBL}
	
	bounds, err := rtreego.NewRect(bottomLeftPoint, rectSize)
	if err != nil {
		return nil, fmt.Errorf("invalid bounding box: %w", err)
	}

	// Search for intersecting items
	results := g.tree.SearchIntersect(bounds)
	
	// Filter results to ensure they're actually within the box
	points := make([]*Point, 0, len(results))
	for _, result := range results {
		item, ok := result.(*spatialItem)
		if !ok || item.Point == nil {
			continue
		}
		
		// Verify point is within the box boundaries
		if item.Lat >= latBL && item.Lat <= latTR &&
			item.Lon >= lonBL && item.Lon <= lonTR {
			points = append(points, item.Point)
		}
	}

	return points, nil
}

// SearchRadius returns all points within the given radius (in km) from the center point
func (g *GeoIndex) SearchRadius(centerLat, centerLon float64, radiusKm float64) ([]*Point, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Convert radius to degrees (approximation)
	deg := (radiusKm / earthRadius) * (180 / math.Pi)
	
	// Create bounding box
	bounds, err := rtreego.NewRect(
		rtreego.Point{centerLat - deg, centerLon - deg},
		[]float64{2 * deg, 2 * deg},
	)
	if err != nil {
		return nil, fmt.Errorf("invalid radius search: %w", err)
	}

	// Search for intersecting items
	results := g.tree.SearchIntersect(bounds)
	
	// Filter by actual distance
	points := make([]*Point, 0, len(results))
	for _, result := range results {
		item, ok := result.(*spatialItem)
		if !ok || item.Point == nil {
			continue
		}
		
		// Calculate actual distance
		dist := haversineDistance(centerLat, centerLon, item.Lat, item.Lon)
		if dist <= radiusKm {
			points = append(points, item.Point)
		}
	}

	return points, nil
}

// NearestNeighbors returns the N nearest points to the given location
func (g *GeoIndex) NearestNeighbors(lat, lon float64, n int) []*Point {
	g.mu.RLock()
	defer g.mu.RUnlock()

	queryPoint := rtreego.Point{lat, lon}
	results := g.tree.NearestNeighbors(n, queryPoint)
	
	points := make([]*Point, len(results))
	for i, result := range results {
		points[i] = result.(*spatialItem).Point
	}

	return points
}

// Size returns the number of points in the index
func (g *GeoIndex) Size() int64 {
	return g.itemCount.Load()
}

// Clear removes all points from the index
func (g *GeoIndex) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()
	
	g.tree = rtreego.NewTree(dimensions, minChildren, maxChildren)
	g.itemCount.Store(0)
}

// SaveToFile saves the index to a file using gob encoding
func (g *GeoIndex) SaveToFile(filename string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Collect all points by searching with a very large bounding box
	var points []*Point
	largeBounds, _ := rtreego.NewRect(rtreego.Point{-90, -180}, []float64{180, 360})
	results := g.tree.SearchIntersect(largeBounds)
	
	for _, result := range results {
		if item, ok := result.(*spatialItem); ok {
			points = append(points, item.Point)
		}
	}

	// Encode points
	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(points); err != nil {
		return fmt.Errorf("failed to encode index: %w", err)
	}

	return nil
}

// LoadFromFile loads the index from a file
func (g *GeoIndex) LoadFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Decode points
	var points []*Point
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&points); err != nil {
		return fmt.Errorf("failed to decode index: %w", err)
	}

	// Clear existing index and re-index points
	g.Clear()
	g.IndexPoints(points)

	return nil
}

// haversineDistance calculates the distance between two lat/lon points in kilometers
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
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