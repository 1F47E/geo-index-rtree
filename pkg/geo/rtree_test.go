package geo

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"
)

func TestIndexAndSearchBox(t *testing.T) {
	index := NewGeoIndex()
	
	// Create test points
	points := []*Point{
		{ID: "1", Lat: 40.7128, Lon: -74.0060},  // New York
		{ID: "2", Lat: 51.5074, Lon: -0.1278},   // London
		{ID: "3", Lat: 48.8566, Lon: 2.3522},    // Paris
		{ID: "4", Lat: 35.6762, Lon: 139.6503},  // Tokyo
		{ID: "5", Lat: -33.8688, Lon: 151.2093}, // Sydney
	}
	
	// Index points
	index.IndexPoints(points)
	
	if index.Size() != int64(len(points)) {
		t.Errorf("Expected %d points, got %d", len(points), index.Size())
	}
	
	// Search box around Europe
	results, err := index.SearchBox(45.0, -5.0, 55.0, 10.0)
	if err != nil {
		t.Fatalf("SearchBox failed: %v", err)
	}
	
	// Should find London and Paris
	if len(results) != 2 {
		t.Errorf("Expected 2 results in Europe box, got %d", len(results))
	}
}

func TestSearchRadius(t *testing.T) {
	index := NewGeoIndex()
	
	// Create points around a center
	centerLat, centerLon := 40.0, -74.0
	points := []*Point{
		{ID: "center", Lat: centerLat, Lon: centerLon},
		{ID: "near", Lat: centerLat + 0.1, Lon: centerLon + 0.1}, // ~15km away
		{ID: "far", Lat: centerLat + 1.0, Lon: centerLon + 1.0},  // ~150km away
	}
	
	index.IndexPoints(points)
	
	// Search within 50km radius
	results, err := index.SearchRadius(centerLat, centerLon, 50.0)
	if err != nil {
		t.Fatalf("SearchRadius failed: %v", err)
	}
	
	// Should find center and near points
	if len(results) != 2 {
		t.Errorf("Expected 2 results within 50km, got %d", len(results))
	}
}

func TestNearestNeighbors(t *testing.T) {
	index := NewGeoIndex()
	
	// Create a grid of points
	var points []*Point
	for i := 0; i < 10; i++ {
		for j := 0; j < 10; j++ {
			points = append(points, &Point{
				ID:  fmt.Sprintf("%d,%d", i, j),
				Lat: float64(i),
				Lon: float64(j),
			})
		}
	}
	
	index.IndexPoints(points)
	
	// Find 5 nearest neighbors to (4.5, 4.5)
	neighbors := index.NearestNeighbors(4.5, 4.5, 5)
	
	if len(neighbors) != 5 {
		t.Errorf("Expected 5 neighbors, got %d", len(neighbors))
	}
	
	// The nearest should be one of the corner points around (4.5, 4.5)
	nearest := neighbors[0]
	dist := math.Sqrt(math.Pow(nearest.Lat-4.5, 2) + math.Pow(nearest.Lon-4.5, 2))
	if dist > 1.0 {
		t.Errorf("Nearest neighbor too far: %f", dist)
	}
}

func TestParallelIndexing(t *testing.T) {
	index := NewGeoIndex()
	
	// Generate many random points
	numPoints := 10000
	points := make([]*Point, numPoints)
	for i := 0; i < numPoints; i++ {
		points[i] = &Point{
			ID:  fmt.Sprintf("p%d", i),
			Lat: rand.Float64()*180 - 90,
			Lon: rand.Float64()*360 - 180,
		}
	}
	
	// Measure indexing time
	start := time.Now()
	index.IndexPoints(points)
	elapsed := time.Since(start)
	
	t.Logf("Indexed %d points in %v", numPoints, elapsed)
	
	if index.Size() != int64(numPoints) {
		t.Errorf("Expected %d points, got %d", numPoints, index.Size())
	}
}

func BenchmarkSearchBox(b *testing.B) {
	index := NewGeoIndex()
	
	// Create 1 million random points
	numPoints := 1000000
	points := make([]*Point, numPoints)
	for i := 0; i < numPoints; i++ {
		points[i] = &Point{
			ID:  fmt.Sprintf("p%d", i),
			Lat: rand.Float64()*180 - 90,
			Lon: rand.Float64()*360 - 180,
		}
	}
	
	index.IndexPoints(points)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		// Random box
		lat1 := rand.Float64()*160 - 80
		lon1 := rand.Float64()*340 - 170
		lat2 := lat1 + rand.Float64()*10
		lon2 := lon1 + rand.Float64()*10
		
		_, _ = index.SearchBox(lat1, lon1, lat2, lon2)
	}
}

func BenchmarkSearchRadius(b *testing.B) {
	index := NewGeoIndex()
	
	// Create 1 million random points
	numPoints := 1000000
	points := make([]*Point, numPoints)
	for i := 0; i < numPoints; i++ {
		points[i] = &Point{
			ID:  fmt.Sprintf("p%d", i),
			Lat: rand.Float64()*180 - 90,
			Lon: rand.Float64()*360 - 180,
		}
	}
	
	index.IndexPoints(points)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		lat := rand.Float64()*180 - 90
		lon := rand.Float64()*360 - 180
		
		_, _ = index.SearchRadius(lat, lon, 50.0)
	}
}

func BenchmarkNearestNeighbors(b *testing.B) {
	index := NewGeoIndex()
	
	// Create 1 million random points
	numPoints := 1000000
	points := make([]*Point, numPoints)
	for i := 0; i < numPoints; i++ {
		points[i] = &Point{
			ID:  fmt.Sprintf("p%d", i),
			Lat: rand.Float64()*180 - 90,
			Lon: rand.Float64()*360 - 180,
		}
	}
	
	index.IndexPoints(points)
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		lat := rand.Float64()*180 - 90
		lon := rand.Float64()*360 - 180
		
		_ = index.NearestNeighbors(lat, lon, 10)
	}
}