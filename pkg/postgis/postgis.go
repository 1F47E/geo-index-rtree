package postgis

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/kass/go-geo-index/pkg/models"
)

type PostGISIndex struct {
	db *sql.DB
}

// NewPostGISIndex creates a new PostGIS connection
func NewPostGISIndex(host, user, password, dbname string, port int) (*PostGISIndex, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	
	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	
	// Set connection pool settings for better performance
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	
	return &PostGISIndex{db: db}, nil
}

// InitSchema creates the necessary tables and indexes
func (p *PostGISIndex) InitSchema() error {
	queries := []string{
		// Enable PostGIS extension
		`CREATE EXTENSION IF NOT EXISTS postgis;`,
		
		// Drop existing table if exists
		`DROP TABLE IF EXISTS geo_points;`,
		
		// Create table with geometry column
		`CREATE TABLE geo_points (
			id TEXT PRIMARY KEY,
			location GEOMETRY(POINT, 4326)
		);`,
	}
	
	for _, query := range queries {
		if _, err := p.db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute query '%s': %w", query, err)
		}
	}
	
	return nil
}

// CreateSpatialIndex creates a GIST index on the geometry column
func (p *PostGISIndex) CreateSpatialIndex() error {
	query := `CREATE INDEX idx_geo_points_location ON geo_points USING GIST(location);`
	
	start := time.Now()
	if _, err := p.db.Exec(query); err != nil {
		return fmt.Errorf("failed to create spatial index: %w", err)
	}
	
	// Analyze table for better query planning
	if _, err := p.db.Exec("ANALYZE geo_points;"); err != nil {
		return fmt.Errorf("failed to analyze table: %w", err)
	}
	
	elapsed := time.Since(start)
	fmt.Printf("Created spatial index in %v\n", elapsed)
	
	return nil
}

// BulkInsertPoints inserts points in batches for better performance
func (p *PostGISIndex) BulkInsertPoints(points []*models.Point) error {
	const batchSize = 10000
	
	// Prepare statement
	stmt, err := p.db.Prepare(`
		INSERT INTO geo_points (id, location) 
		VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), 4326))
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()
	
	// Begin transaction
	tx, err := p.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	
	txStmt := tx.Stmt(stmt)
	
	// Insert in batches
	for i := 0; i < len(points); i++ {
		point := points[i]
		_, err := txStmt.Exec(point.ID, point.Location.Lon, point.Location.Lat)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to insert point %s: %w", point.ID, err)
		}
		
		// Commit batch
		if (i+1)%batchSize == 0 {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit batch: %w", err)
			}
			
			// Start new transaction
			tx, err = p.db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin new transaction: %w", err)
			}
			txStmt = tx.Stmt(stmt)
		}
	}
	
	// Commit final batch
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit final batch: %w", err)
	}
	
	return nil
}

// QueryBox performs a bounding box query
func (p *PostGISIndex) QueryBox(box models.BoundingBox) ([]*models.Point, error) {
	query := `
		SELECT id, ST_Y(location) as lat, ST_X(location) as lon
		FROM geo_points
		WHERE location && ST_MakeEnvelope($1, $2, $3, $4, 4326)
	`
	
	rows, err := p.db.Query(query, 
		box.BottomLeft.Lon, box.BottomLeft.Lat,
		box.TopRight.Lon, box.TopRight.Lat)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()
	
	var results []*models.Point
	for rows.Next() {
		var id string
		var lat, lon float64
		
		if err := rows.Scan(&id, &lat, &lon); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}
		
		results = append(results, &models.Point{
			ID: id,
			Location: &models.Location{
				Lat: lat,
				Lon: lon,
			},
		})
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	
	return results, nil
}

// Count returns the number of points in the database
func (p *PostGISIndex) Count() (int64, error) {
	var count int64
	err := p.db.QueryRow("SELECT COUNT(*) FROM geo_points").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count points: %w", err)
	}
	return count, nil
}

// GetDatabaseStats returns database size and table statistics
func (p *PostGISIndex) GetDatabaseStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Get database size
	var dbSize string
	err := p.db.QueryRow(`
		SELECT pg_size_pretty(pg_database_size('geodb'))
	`).Scan(&dbSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get database size: %w", err)
	}
	stats["database_size"] = dbSize
	
	// Get table size
	var tableSize, indexSize string
	err = p.db.QueryRow(`
		SELECT 
			pg_size_pretty(pg_total_relation_size('geo_points')) as total_size,
			pg_size_pretty(pg_indexes_size('geo_points')) as index_size
	`).Scan(&tableSize, &indexSize)
	if err != nil {
		// Table might not exist yet
		stats["table_size"] = "0 bytes"
		stats["index_size"] = "0 bytes"
	} else {
		stats["table_size"] = tableSize
		stats["index_size"] = indexSize
	}
	
	// Get row count
	count, _ := p.Count()
	stats["row_count"] = count
	
	return stats, nil
}

// Close closes the database connection
func (p *PostGISIndex) Close() error {
	return p.db.Close()
}