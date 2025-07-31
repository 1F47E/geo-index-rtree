package rtree

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/1F47E/geo-index-rtree/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGeoIndex(t *testing.T) {
	index := NewGeoIndex()
	assert.NotNil(t, index)
	assert.NotNil(t, index.tree)
	assert.Equal(t, int64(0), index.Count())
}

func TestIndexPoints(t *testing.T) {
	index := NewGeoIndex()
	
	points := []*models.Point{
		{ID: "1", Location: &models.Location{Lat: 37.7749, Lon: -122.4194}}, // San Francisco
		{ID: "2", Location: &models.Location{Lat: 34.0522, Lon: -118.2437}}, // Los Angeles
		{ID: "3", Location: &models.Location{Lat: 40.7128, Lon: -74.0060}},  // New York
		{ID: "4", Location: nil}, // Point without location
	}
	
	err := index.IndexPoints(points)
	assert.NoError(t, err)
	assert.Equal(t, int64(3), index.Count()) // Only 3 points have locations
}

func TestQueryBox(t *testing.T) {
	index := NewGeoIndex()
	
	// Create points in California
	points := []*models.Point{
		{ID: "SF", Location: &models.Location{Lat: 37.7749, Lon: -122.4194}},    // San Francisco
		{ID: "LA", Location: &models.Location{Lat: 34.0522, Lon: -118.2437}},    // Los Angeles
		{ID: "SD", Location: &models.Location{Lat: 32.7157, Lon: -117.1611}},    // San Diego
		{ID: "NYC", Location: &models.Location{Lat: 40.7128, Lon: -74.0060}},    // New York (outside)
		{ID: "CHI", Location: &models.Location{Lat: 41.8781, Lon: -87.6298}},    // Chicago (outside)
	}
	
	err := index.IndexPoints(points)
	require.NoError(t, err)
	
	// Query box covering California
	box := models.BoundingBox{
		BottomLeft: models.Location{Lat: 32.0, Lon: -125.0},
		TopRight:   models.Location{Lat: 42.0, Lon: -114.0},
	}
	
	results, err := index.QueryBox(box)
	assert.NoError(t, err)
	assert.Len(t, results, 3) // SF, LA, SD
	
	// Verify results
	resultIDs := make(map[string]bool)
	for _, p := range results {
		resultIDs[p.ID] = true
	}
	
	assert.True(t, resultIDs["SF"])
	assert.True(t, resultIDs["LA"])
	assert.True(t, resultIDs["SD"])
	assert.False(t, resultIDs["NYC"])
	assert.False(t, resultIDs["CHI"])
}

func TestQueryRadius(t *testing.T) {
	index := NewGeoIndex()
	
	// Create points around San Francisco
	sfLat, sfLon := 37.7749, -122.4194
	points := []*models.Point{
		{ID: "SF", Location: &models.Location{Lat: sfLat, Lon: sfLon}},
		{ID: "Oakland", Location: &models.Location{Lat: 37.8044, Lon: -122.2712}},    // ~13km
		{ID: "San Jose", Location: &models.Location{Lat: 37.3382, Lon: -121.8863}},   // ~48km
		{ID: "Sacramento", Location: &models.Location{Lat: 38.5816, Lon: -121.4944}}, // ~120km
		{ID: "LA", Location: &models.Location{Lat: 34.0522, Lon: -118.2437}},         // ~560km
	}
	
	err := index.IndexPoints(points)
	require.NoError(t, err)
	
	testCases := []struct {
		name     string
		radius   float64
		expected []string
	}{
		{"10km radius", 10, []string{"SF"}},
		{"20km radius", 20, []string{"SF", "Oakland"}},
		{"80km radius", 80, []string{"SF", "Oakland", "San Jose"}},
		{"150km radius", 150, []string{"SF", "Oakland", "San Jose", "Sacramento"}},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			center := models.Location{Lat: sfLat, Lon: sfLon}
			results, err := index.QueryRadius(center, tc.radius)
			assert.NoError(t, err)
			assert.Len(t, results, len(tc.expected))
			
			resultIDs := make(map[string]bool)
			for _, p := range results {
				resultIDs[p.ID] = true
			}
			
			for _, expectedID := range tc.expected {
				assert.True(t, resultIDs[expectedID], "Expected %s in results", expectedID)
			}
		})
	}
}

func TestNearestNeighbors(t *testing.T) {
	index := NewGeoIndex()
	
	// Create points
	points := []*models.Point{
		{ID: "1", Location: &models.Location{Lat: 37.7749, Lon: -122.4194}},
		{ID: "2", Location: &models.Location{Lat: 37.7849, Lon: -122.4094}},
		{ID: "3", Location: &models.Location{Lat: 37.7649, Lon: -122.4294}},
		{ID: "4", Location: &models.Location{Lat: 37.8049, Lon: -122.3994}},
		{ID: "5", Location: &models.Location{Lat: 37.7549, Lon: -122.4394}},
	}
	
	err := index.IndexPoints(points)
	require.NoError(t, err)
	
	// Query nearest neighbors
	center := models.Location{Lat: 37.7749, Lon: -122.4194}
	results := index.NearestNeighbors(center, 3)
	
	assert.Len(t, results, 3)
	// First result should be the center point itself
	assert.Equal(t, "1", results[0].ID)
}

func TestPersistence(t *testing.T) {
	// Create and populate index
	index1 := NewGeoIndex()
	points := generateRandomPoints(100)
	err := index1.IndexPoints(points)
	require.NoError(t, err)
	
	// Save to file
	tempFile := fmt.Sprintf("/tmp/test_index_%d.gob", time.Now().UnixNano())
	err = index1.SaveToFile(tempFile)
	require.NoError(t, err)
	
	// Load into new index
	index2 := NewGeoIndex()
	err = index2.LoadFromFile(tempFile)
	require.NoError(t, err)
	
	// Verify counts match
	assert.Equal(t, index1.Count(), index2.Count())
	
	// Verify query results match
	box := models.BoundingBox{
		BottomLeft: models.Location{Lat: 30, Lon: -120},
		TopRight:   models.Location{Lat: 40, Lon: -110},
	}
	
	results1, err := index1.QueryBox(box)
	require.NoError(t, err)
	
	results2, err := index2.QueryBox(box)
	require.NoError(t, err)
	
	assert.Equal(t, len(results1), len(results2))
}

func TestConcurrentQueries(t *testing.T) {
	index := NewGeoIndex()
	points := generateRandomPoints(10000)
	err := index.IndexPoints(points)
	require.NoError(t, err)
	
	// Run concurrent queries
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- true }()
			
			// Random query type
			switch rand.Intn(3) {
			case 0: // Box query
				box := models.BoundingBox{
					BottomLeft: models.Location{Lat: rand.Float64()*10 + 30, Lon: rand.Float64()*10 - 120},
					TopRight:   models.Location{Lat: rand.Float64()*10 + 40, Lon: rand.Float64()*10 - 110},
				}
				_, err := index.QueryBox(box)
				assert.NoError(t, err)
				
			case 1: // Radius query
				center := models.Location{Lat: rand.Float64()*20 + 30, Lon: rand.Float64()*40 - 120}
				_, err := index.QueryRadius(center, rand.Float64()*100+10)
				assert.NoError(t, err)
				
			case 2: // Nearest neighbors
				center := models.Location{Lat: rand.Float64()*20 + 30, Lon: rand.Float64()*40 - 120}
				results := index.NearestNeighbors(center, rand.Intn(50)+1)
				assert.NotNil(t, results)
			}
		}()
	}
	
	// Wait for all queries to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestDistance(t *testing.T) {
	testCases := []struct {
		name     string
		lat1     float64
		lon1     float64
		lat2     float64
		lon2     float64
		expected float64
		delta    float64
	}{
		{
			name:     "Same point",
			lat1:     37.7749, lon1: -122.4194,
			lat2:     37.7749, lon2: -122.4194,
			expected: 0,
			delta:    0.01,
		},
		{
			name:     "SF to Oakland",
			lat1:     37.7749, lon1: -122.4194,
			lat2:     37.8044, lon2: -122.2712,
			expected: 13.0, // Approximately 13km
			delta:    1.0,
		},
		{
			name:     "SF to LA",
			lat1:     37.7749, lon1: -122.4194,
			lat2:     34.0522, lon2: -118.2437,
			expected: 559.0, // Approximately 559km
			delta:    5.0,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dist := Distance(tc.lat1, tc.lon1, tc.lat2, tc.lon2)
			assert.InDelta(t, tc.expected, dist, tc.delta)
		})
	}
}

// Helper function to generate random points
func generateRandomPoints(n int) []*models.Point {
	points := make([]*models.Point, n)
	for i := 0; i < n; i++ {
		points[i] = &models.Point{
			ID: fmt.Sprintf("point_%d", i),
			Location: &models.Location{
				Lat: rand.Float64()*20 + 30,    // 30-50
				Lon: rand.Float64()*40 - 120,   // -120 to -80
			},
		}
	}
	return points
}

// Benchmarks
func BenchmarkIndexPoints(b *testing.B) {
	sizes := []int{1000, 10000, 100000}
	
	for _, size := range sizes {
		b.Run(fmt.Sprintf("%d_points", size), func(b *testing.B) {
			points := generateRandomPoints(size)
			b.ResetTimer()
			
			for i := 0; i < b.N; i++ {
				index := NewGeoIndex()
				_ = index.IndexPoints(points)
			}
		})
	}
}

func BenchmarkQueryBox(b *testing.B) {
	index := NewGeoIndex()
	points := generateRandomPoints(100000)
	_ = index.IndexPoints(points)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		box := models.BoundingBox{
			BottomLeft: models.Location{Lat: 35, Lon: -115},
			TopRight:   models.Location{Lat: 40, Lon: -110},
		}
		_, _ = index.QueryBox(box)
	}
}

func BenchmarkQueryRadius(b *testing.B) {
	index := NewGeoIndex()
	points := generateRandomPoints(100000)
	_ = index.IndexPoints(points)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		center := models.Location{Lat: 37.5, Lon: -112.5}
		_, _ = index.QueryRadius(center, 50)
	}
}

func BenchmarkNearestNeighbors(b *testing.B) {
	index := NewGeoIndex()
	points := generateRandomPoints(100000)
	_ = index.IndexPoints(points)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		center := models.Location{Lat: 37.5, Lon: -112.5}
		_ = index.NearestNeighbors(center, 10)
	}
}