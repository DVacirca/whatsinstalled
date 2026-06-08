package store

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// Package represents an installed package.
type Package struct {
	ID              int64
	Name            string
	Version         string
	Source          string // apt | snap | npm | pip | conda
	Location        string // system or absolute path
	SizeBytes       *int64
	Description     string
	InstalledAt     string
	AutoInstalled   bool
	User            string // who installed it
	UpdatedAt       time.Time
	LastUsed        *time.Time // access time of package directory
	Embedding       string     // JSON array of float64
}

// Store wraps the SQLite connection.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path.
func Open(path string) (*Store, error) {
	connStr := path + "?_busy_timeout=5000"
	db, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// WAL mode allows readers while a writer holds the lock — essential for
	// background scan + UI queries on the same DB.
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate db: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS packages (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			version TEXT,
			source TEXT NOT NULL,
			location TEXT NOT NULL,
			size_bytes INTEGER,
			description TEXT,
			installed_at TEXT,
			auto_installed INTEGER DEFAULT 0,
			user TEXT,
			updated_at INTEGER,
			last_used INTEGER,
			embedding TEXT
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_pkg ON packages(name, source, location);
	`)
	if err != nil {
		return err
	}

	// Migrate existing databases: add columns if missing
	_, _ = db.Exec(`ALTER TABLE packages ADD COLUMN user TEXT`)
	_, _ = db.Exec(`ALTER TABLE packages ADD COLUMN last_used INTEGER`)
	_, _ = db.Exec(`ALTER TABLE packages ADD COLUMN embedding TEXT`)

	// Enrichment cache table for API responses
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS enrichment_cache (
			name TEXT NOT NULL,
			source TEXT NOT NULL,
			description TEXT NOT NULL,
			fetched_at INTEGER NOT NULL,
			PRIMARY KEY (name, source)
		);
	`)
	return nil
}

// Upsert inserts or updates a package.
func (s *Store) Upsert(p Package) error {
	auto := 0
	if p.AutoInstalled {
		auto = 1
	}
	now := time.Now().UnixMilli()
	var lastUsed *int64
	if p.LastUsed != nil {
		v := p.LastUsed.UnixMilli()
		lastUsed = &v
	}
	_, err := s.db.Exec(`
		INSERT INTO packages (name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name, source, location) DO UPDATE SET
			version=excluded.version,
			size_bytes=excluded.size_bytes,
			description=excluded.description,
			installed_at=excluded.installed_at,
			auto_installed=excluded.auto_installed,
			user=excluded.user,
			updated_at=excluded.updated_at,
			last_used=excluded.last_used;
	`, p.Name, p.Version, p.Source, p.Location, p.SizeBytes, p.Description, p.InstalledAt, auto, p.User, now, lastUsed)
	return err
}

// List returns packages matching an optional source filter ("" for all).
func (s *Store) List(sourceFilter string) ([]Package, error) {
	var rows *sql.Rows
	var err error
	if sourceFilter == "" {
		rows, err = s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used FROM packages ORDER BY name COLLATE NOCASE`)
	} else {
		rows, err = s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used FROM packages WHERE source = ? ORDER BY name COLLATE NOCASE`, sourceFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("list packages: %w", err)
	}
	defer rows.Close()

	return scanRows(rows)
}

// Search returns packages matching a name substring within an optional source filter.
func (s *Store) Search(query, sourceFilter string) ([]Package, error) {
	query = "%" + query + "%"
	var rows *sql.Rows
	var err error
	if sourceFilter == "" {
		rows, err = s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used FROM packages WHERE name LIKE ? ORDER BY name COLLATE NOCASE`, query)
	} else {
		rows, err = s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used FROM packages WHERE name LIKE ? AND source = ? ORDER BY name COLLATE NOCASE`, query, sourceFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("search packages: %w", err)
	}
	defer rows.Close()

	return scanRows(rows)
}

// Delete removes a package by name, source, and location.
func (s *Store) Delete(name, source, location string) error {
	_, err := s.db.Exec(`DELETE FROM packages WHERE name = ? AND source = ? AND location = ?`, name, source, location)
	return err
}

// PurgeStale removes packages whose updated_at is older than the cutoff.
func (s *Store) PurgeStale(cutoff time.Time) error {
	_, err := s.db.Exec(`DELETE FROM packages WHERE updated_at < ?`, cutoff.UnixMilli())
	return err
}

// CountBySource returns package counts per source.
func (s *Store) CountBySource() (map[string]int, int, error) {
	rows, err := s.db.Query(`SELECT source, COUNT(*) FROM packages GROUP BY source`)
	if err != nil {
		return nil, 0, fmt.Errorf("count by source: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	total := 0
	for rows.Next() {
		var src string
		var c int
		if err := rows.Scan(&src, &c); err != nil {
			return nil, 0, err
		}
		counts[src] = c
		total += c
	}
	return counts, total, rows.Err()
}

// UpdateEmbedding stores the embedding JSON for a package.
func (s *Store) UpdateEmbedding(id int64, embeddingJSON string) error {
	_, err := s.db.Exec(`UPDATE packages SET embedding = ? WHERE id = ?`, embeddingJSON, id)
	return err
}

// ListWithoutEmbeddings returns packages that have no embedding cached.
func (s *Store) ListWithoutEmbeddings() ([]Package, error) {
	rows, err := s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used FROM packages WHERE embedding IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("list without embeddings: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// ListWithEmbeddings returns all packages with their embeddings.
func (s *Store) ListWithEmbeddings() ([]Package, error) {
	rows, err := s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used, embedding FROM packages`)
	if err != nil {
		return nil, fmt.Errorf("list with embeddings: %w", err)
	}
	defer rows.Close()

	var pkgs []Package
	for rows.Next() {
		var p Package
		var size sql.NullInt64
		var auto int
		var updated int64
		var user sql.NullString
		var lastUsed sql.NullInt64
		var embedding sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Source, &p.Location, &size, &p.Description, &p.InstalledAt, &auto, &user, &updated, &lastUsed, &embedding); err != nil {
			return nil, err
		}
		if size.Valid {
			p.SizeBytes = &size.Int64
		}
		if user.Valid {
			p.User = user.String
		}
		if lastUsed.Valid {
			t := time.UnixMilli(lastUsed.Int64)
			p.LastUsed = &t
		}
		p.AutoInstalled = auto != 0
		p.UpdatedAt = time.UnixMilli(updated)
		pkgs = append(pkgs, p)
	}
	return pkgs, rows.Err()
}

// ListWithoutDescriptions returns packages that have no description.
func (s *Store) ListWithoutDescriptions(sourceFilter string) ([]Package, error) {
	var rows *sql.Rows
	var err error
	if sourceFilter == "" {
		rows, err = s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used FROM packages WHERE description IS NULL OR description = '' ORDER BY name COLLATE NOCASE`)
	} else {
		rows, err = s.db.Query(`SELECT id, name, version, source, location, size_bytes, description, installed_at, auto_installed, user, updated_at, last_used FROM packages WHERE (description IS NULL OR description = '') AND source = ? ORDER BY name COLLATE NOCASE`, sourceFilter)
	}
	if err != nil {
		return nil, fmt.Errorf("list without descriptions: %w", err)
	}
	defer rows.Close()
	return scanRows(rows)
}

// UpdateDescription updates the description for a single package.
func (s *Store) UpdateDescription(id int64, description string) error {
	_, err := s.db.Exec(`UPDATE packages SET description = ? WHERE id = ?`, description, id)
	return err
}

// UpdateManyDescriptions updates descriptions for multiple packages in a single transaction.
func (s *Store) UpdateManyDescriptions(pkgs []Package) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE packages SET description = ? WHERE id = ?`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, p := range pkgs {
		if p.Description == "" {
			continue
		}
		if _, err := stmt.Exec(p.Description, p.ID); err != nil {
			return fmt.Errorf("update desc %s: %w", p.Name, err)
		}
	}
	return tx.Commit()
}

// CountWithoutDescriptions returns the number of packages missing descriptions.
func (s *Store) CountWithoutDescriptions() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM packages WHERE description IS NULL OR description = ''`).Scan(&count)
	return count, err
}

// GetEnrichmentCache returns the underlying database connection for the enrichment cache.
func (s *Store) GetEnrichmentCache() *sql.DB {
	return s.db
}

func scanRows(rows *sql.Rows) ([]Package, error) {
	var pkgs []Package
	for rows.Next() {
		var p Package
		var size sql.NullInt64
		var auto int
		var updated int64
		var user sql.NullString
		var lastUsed sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &p.Source, &p.Location, &size, &p.Description, &p.InstalledAt, &auto, &user, &updated, &lastUsed); err != nil {
			return nil, err
		}
		if size.Valid {
			p.SizeBytes = &size.Int64
		}
		if user.Valid {
			p.User = user.String
		}
		if lastUsed.Valid {
			t := time.UnixMilli(lastUsed.Int64)
			p.LastUsed = &t
		}
		p.AutoInstalled = auto != 0
		p.UpdatedAt = time.UnixMilli(updated)
		pkgs = append(pkgs, p)
	}
	return pkgs, rows.Err()
}

// DBPath returns the default database path.
func DBPath() string {
	if p := os.Getenv("INSTALLR_DB"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "installr.db"
	}
	return home + "/.installr.db"
}
