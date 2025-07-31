package rtree

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/kass/go-geo-index/pkg/models"
)

// IndexData represents the serializable form of the geo index
type IndexData struct {
	Points []*models.Point `json:"points"`
	Count  int64          `json:"count"`
}

// SaveToFile saves the index to a binary file
func (g *GeoIndex) SaveToFile(filename string) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Extract all points from the tree
	// Since rtreego doesn't provide a way to iterate all items,
	// we'll use a large bounding box to get all points
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