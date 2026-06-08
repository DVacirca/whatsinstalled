package enrich

import (
	"database/sql"
	"fmt"
	"time"
)

// Cache stores enrichment results in SQLite to avoid repeated API calls.
type Cache struct {
	db *sql.DB
}

// NewCache creates a cache backed by the given SQLite connection.
// The caller must ensure the enrichment_cache table exists (see InitCacheTable).
func NewCache(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// InitCacheTable creates the enrichment_cache table if missing.
func InitCacheTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS enrichment_cache (
			name TEXT NOT NULL,
			source TEXT NOT NULL,
			description TEXT NOT NULL,
			fetched_at INTEGER NOT NULL,
			PRIMARY KEY (name, source)
		);
	`)
	return err
}

// Get retrieves a cached description if it exists and is not expired.
func (c *Cache) Get(name, source string, ttl time.Duration) (string, bool) {
	var desc string
	var fetchedAt int64
	err := c.db.QueryRow(
		`SELECT description, fetched_at FROM enrichment_cache WHERE name = ? AND source = ?`,
		name, source,
	).Scan(&desc, &fetchedAt)
	if err != nil {
		return "", false
	}
	if time.Since(time.Unix(fetchedAt, 0)) > ttl {
		return "", false
	}
	return desc, true
}

// Set stores a description in the cache.
func (c *Cache) Set(name, source, description string) error {
	_, err := c.db.Exec(`
		INSERT INTO enrichment_cache (name, source, description, fetched_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name, source) DO UPDATE SET
			description = excluded.description,
			fetched_at = excluded.fetched_at;
	`, name, source, description, time.Now().Unix())
	return err
}

// BatchSet stores multiple descriptions in a single transaction.
func (c *Cache) BatchSet(items []CacheItem) error {
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("begin cache tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO enrichment_cache (name, source, description, fetched_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(name, source) DO UPDATE SET
			description = excluded.description,
			fetched_at = excluded.fetched_at;
	`)
	if err != nil {
		return fmt.Errorf("prepare cache stmt: %w", err)
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, item := range items {
		if _, err := stmt.Exec(item.Name, item.Source, item.Description, now); err != nil {
			return fmt.Errorf("cache set %s/%s: %w", item.Name, item.Source, err)
		}
	}
	return tx.Commit()
}

// CacheItem is a single entry for batch cache operations.
type CacheItem struct {
	Name        string
	Source      string
	Description string
}

// Prune removes entries older than the given TTL.
func (c *Cache) Prune(ttl time.Duration) error {
	cutoff := time.Now().Add(-ttl).Unix()
	_, err := c.db.Exec(`DELETE FROM enrichment_cache WHERE fetched_at < ?`, cutoff)
	return err
}

// Stats returns the total number of cached entries.
func (c *Cache) Stats() (int, error) {
	var count int
	err := c.db.QueryRow(`SELECT COUNT(*) FROM enrichment_cache`).Scan(&count)
	return count, err
}
