package main

import (
	"fmt"
	"log"

	"github.com/kass/go-geo-index/pkg/models"
	"github.com/kass/go-geo-index/pkg/rtree"
)

func main() {
	// Create a new geo index
	index := rtree.NewGeoIndex()

	// Sample points for major US cities
	cities := []*models.Point{
		{ID: "NYC", Location: &models.Location{Lat: 40.7128, Lon: -74.0060}},
		{ID: "LAX", Location: &models.Location{Lat: 34.0522, Lon: -118.2437}},
		{ID: "CHI", Location: &models.Location{Lat: 41.8781, Lon: -87.6298}},
		{ID: "HOU", Location: &models.Location{Lat: 29.7604, Lon: -95.3698}},
		{ID: "PHX", Location: &models.Location{Lat: 33.4484, Lon: -112.0740}},
		{ID: "PHL", Location: &models.Location{Lat: 39.9526, Lon: -75.1652}},
		{ID: "SAT", Location: &models.Location{Lat: 29.4241, Lon: -98.4936}},
		{ID: "SDG", Location: &models.Location{Lat: 32.7157, Lon: -117.1611}},
		{ID: "DAL", Location: &models.Location{Lat: 32.7767, Lon: -96.7970}},
		{ID: "SJC", Location: &models.Location{Lat: 37.3382, Lon: -121.8863}},
		{ID: "AUS", Location: &models.Location{Lat: 30.2672, Lon: -97.7431}},
		{ID: "JAX", Location: &models.Location{Lat: 30.3322, Lon: -81.6557}},
		{ID: "SFO", Location: &models.Location{Lat: 37.7749, Lon: -122.4194}},
		{ID: "CLB", Location: &models.Location{Lat: 39.9612, Lon: -82.9988}},
		{ID: "CLT", Location: &models.Location{Lat: 35.2271, Lon: -80.8431}},
	}

	// Index the cities
	if err := index.IndexPoints(cities); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Indexed %d cities\n\n", index.Count())

	// Example 1: Find cities in California (bounding box)
	fmt.Println("=== Cities in California (Bounding Box) ===")
	californiaBounds := models.BoundingBox{
		BottomLeft: models.Location{Lat: 32.5, Lon: -124.5},
		TopRight:   models.Location{Lat: 42.0, Lon: -114.0},
	}
	
	results, err := index.QueryBox(californiaBounds)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Found %d cities in California:\n", len(results))
	for _, city := range results {
		fmt.Printf("  - %s: (%.4f, %.4f)\n", city.ID, city.Location.Lat, city.Location.Lon)
	}

	// Example 2: Find cities within 500km of Dallas
	fmt.Println("\n=== Cities within 500km of Dallas ===")
	dallasLocation := models.Location{Lat: 32.7767, Lon: -96.7970}
	
	results, err = index.QueryRadius(dallasLocation, 500)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Found %d cities within 500km of Dallas:\n", len(results))
	for _, city := range results {
		distance := rtree.Distance(
			dallasLocation.Lat, dallasLocation.Lon,
			city.Location.Lat, city.Location.Lon,
		)
		fmt.Printf("  - %s: %.1f km away\n", city.ID, distance)
	}

	// Example 3: Find 5 nearest cities to Denver
	fmt.Println("\n=== 5 Nearest Cities to Denver ===")
	denverLocation := models.Location{Lat: 39.7392, Lon: -104.9903}
	
	nearest := index.NearestNeighbors(denverLocation, 5)
	fmt.Printf("Found %d nearest cities to Denver:\n", len(nearest))
	for i, city := range nearest {
		distance := rtree.Distance(
			denverLocation.Lat, denverLocation.Lon,
			city.Location.Lat, city.Location.Lon,
		)
		fmt.Printf("  %d. %s: %.1f km away\n", i+1, city.ID, distance)
	}

	// Save the index
	fmt.Println("\n=== Saving Index ===")
	if err := index.SaveToFile("cities.gob"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Index saved to cities.gob")

	// Load the index
	fmt.Println("\n=== Loading Index ===")
	newIndex := rtree.NewGeoIndex()
	if err := newIndex.LoadFromFile("cities.gob"); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Loaded index with %d points\n", newIndex.Count())
}