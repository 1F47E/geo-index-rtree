package rtree

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/1F47E/geo-index-rtree/pkg/models"
)

// IndexData represents the serializable form of the geo index
type IndexData struct {
	Points []*models.Point `json:"points"`
	Count  int64          `json:"count"`
}

// SaveToFile saves the index to a binary file
func (g *GeoIndex) SaveToFile(filename string) error {
	g.mu.RLock()
	
	// Extract all points from all partitions
	// We need to unlock before calling QueryBox to avoid deadlock
	g.mu.RUnlock()
	
	largeBounds := models.BoundingBox{
		BottomLeft: models.Location{Lat: -90, Lon: -180},
		TopRight:   models.Location{Lat: 90, Lon: 180},
	}
	
	points, err := g.QueryBox(largeBounds)
	if err != nil {
		return fmt.Errorf("failed to extract points: %w", err)
	}

	data := IndexData{
		Points: points,
		Count:  g.itemCount.Load(),
	}

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode data: %w", err)
	}

	return nil
}

// LoadFromFile loads the index from a binary file
func (g *GeoIndex) LoadFromFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var data IndexData
	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&data); err != nil {
		return fmt.Errorf("failed to decode data: %w", err)
	}

	// Clear existing index and rebuild
	g.Clear()
	if err := g.IndexPoints(data.Points); err != nil {
		return fmt.Errorf("failed to index points: %w", err)
	}

	return nil
}