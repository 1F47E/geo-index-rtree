package models

// Location represents a geographic location with latitude and longitude
type Location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Point represents a geo point with an ID and location
type Point struct {
	ID       string    `json:"id"`
	Location *Location `json:"location"`
}

// BoundingBox represents a rectangular area defined by two corners
type BoundingBox struct {
	BottomLeft Location
	TopRight   Location
}