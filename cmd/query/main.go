package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/1F47E/geo-index-rtree/pkg/models"
	"github.com/1F47E/geo-index-rtree/pkg/rtree"
)

func main() {
	var (
		indexFile = flag.String("i", "data/index.gob", "Index file path")
		queryType = flag.String("t", "box", "Query type: box, radius, nearest")
		// Box query parameters
		minLat = flag.Float64("min-lat", 0, "Minimum latitude (box query)")
		maxLat = flag.Float64("max-lat", 0, "Maximum latitude (box query)")
		minLon = flag.Float64("min-lon", 0, "Minimum longitude (box query)")
		maxLon = flag.Float64("max-lon", 0, "Maximum longitude (box query)")
		// Radius query parameters
		centerLat = flag.Float64("lat", 0, "Center latitude (radius/nearest query)")
		centerLon = flag.Float64("lon", 0, "Center longitude (radius/nearest query)")
		radius    = flag.Float64("radius", 10, "Radius in km (radius query)")
		// Nearest query parameters
		k = flag.Int("k", 10, "Number of nearest neighbors (nearest query)")
		// Output format
		outputJSON = flag.Bool("json", false, "Output results as JSON")
		limit      = flag.Int("limit", 100, "Maximum number of results to display")
	)
	flag.Parse()

	// Load index
	log.Printf("Loading index from %s...\n", *indexFile)
	index := rtree.NewGeoIndex()
	if err := index.LoadFromFile(*indexFile); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}
	log.Printf("Index loaded with %d points\n", index.Count())

	var results []*models.Point
	var err error

	switch *queryType {
	case "box":
		if *minLat == 0 && *maxLat == 0 && *minLon == 0 && *maxLon == 0 {
			log.Fatal("Box query requires --min-lat, --max-lat, --min-lon, --max-lon")
		}
		box := models.BoundingBox{
			BottomLeft: models.Location{Lat: *minLat, Lon: *minLon},
			TopRight:   models.Location{Lat: *maxLat, Lon: *maxLon},
		}
		results, err = index.QueryBox(box)
		if err != nil {
			log.Fatalf("Box query failed: %v", err)
		}
		log.Printf("Box query found %d points\n", len(results))

	case "radius":
		if *centerLat == 0 && *centerLon == 0 {
			log.Fatal("Radius query requires --lat and --lon for center point")
		}
		center := models.Location{Lat: *centerLat, Lon: *centerLon}
		results, err = index.QueryRadius(center, *radius)
		if err != nil {
			log.Fatalf("Radius query failed: %v", err)
		}
		log.Printf("Radius query (%.2f km) found %d points\n", *radius, len(results))

	case "nearest":
		if *centerLat == 0 && *centerLon == 0 {
			log.Fatal("Nearest query requires --lat and --lon for center point")
		}
		center := models.Location{Lat: *centerLat, Lon: *centerLon}
		results = index.NearestNeighbors(center, *k)
		log.Printf("Found %d nearest neighbors\n", len(results))

	default:
		log.Fatalf("Unknown query type: %s", *queryType)
	}

	// Limit results if needed
	if len(results) > *limit {
		log.Printf("Showing first %d results (use --limit to see more)\n", *limit)
		results = results[:*limit]
	}

	// Output results
	if *outputJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(results); err != nil {
			log.Fatalf("Failed to encode results: %v", err)
		}
	} else {
		for i, point := range results {
			if *queryType == "radius" || *queryType == "nearest" {
				dist := rtree.Distance(*centerLat, *centerLon, 
					point.Location.Lat, point.Location.Lon)
				fmt.Printf("%d. %s: (%.6f, %.6f) - %.2f km\n", 
					i+1, point.ID, point.Location.Lat, point.Location.Lon, dist)
			} else {
				fmt.Printf("%d. %s: (%.6f, %.6f)\n", 
					i+1, point.ID, point.Location.Lat, point.Location.Lon)
			}
		}
	}
}