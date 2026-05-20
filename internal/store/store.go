// Package store is the persistence layer for unimatrix.
//
// One Store wraps a SQLite database. All schema migrations live in
// migrations/*.sql and are applied at Open() time, tracked via the
// schema_migrations table.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrDroneExists is returned by RegisterDrone when the designation is taken.
var ErrDroneExists = errors.New("drone designation already assimilated")

// ErrAuthFailed is returned by Authenticate on either unknown designation
// or wrong access code. Same error so we don't leak which.
var ErrAuthFailed = errors.New("authentication failed")

type Store struct {
	db *sql.DB
}

type Drone struct {
	ID          int64
	Designation string
	CreatedAt   time.Time
}

type Transmission struct {
	ID          int64
	DroneID     int64
	Designation string // joined from drones
	Body        string
	CreatedAt   time.Time
}

// Open opens or creates the SQLite database at dsn and applies any
// pending migrations. dsn is a path; the driver appends pragmas to
// enable WAL and foreign keys.
func Open(dsn string) (*Store, error) {
	// modernc.org/sqlite accepts URI-style pragmas via _pragma=...
	uri := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dsn)
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	// SQLite is happiest with a single writer. Multiple readers are fine.
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		name        TEXT PRIMARY KEY,
		applied_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var seen int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name,
		).Scan(&seen); err != nil {
			return err
		}
		if seen > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(name) VALUES (?)`, name,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// RegisterDrone creates a new drone with a bcrypt-hashed access code.
// Returns ErrDroneExists if the designation is taken.
func (s *Store) RegisterDrone(designation, accessCode string) (*Drone, error) {
	designation = strings.TrimSpace(designation)
	if designation == "" || accessCode == "" {
		return nil, errors.New("designation and access code are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(accessCode), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		`INSERT INTO drones(designation, password_hash) VALUES (?, ?)`,
		designation, string(hash),
	)
	if err != nil {
		// modernc.org/sqlite returns a constraint error with a recognizable string.
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDroneExists
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.DroneByID(id)
}

// Authenticate verifies the access code for a designation. Returns
// ErrAuthFailed on any failure (unknown drone or wrong code) so the
// caller cannot distinguish.
func (s *Store) Authenticate(designation, accessCode string) (*Drone, error) {
	var (
		id   int64
		hash string
		ts   time.Time
	)
	err := s.db.QueryRow(
		`SELECT id, password_hash, created_at FROM drones WHERE designation = ?`,
		designation,
	).Scan(&id, &hash, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAuthFailed
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(accessCode)); err != nil {
		return nil, ErrAuthFailed
	}
	return &Drone{ID: id, Designation: designation, CreatedAt: ts}, nil
}

// DroneByID looks up a drone by primary key. Returns nil, nil if not found.
func (s *Store) DroneByID(id int64) (*Drone, error) {
	var d Drone
	err := s.db.QueryRow(
		`SELECT id, designation, created_at FROM drones WHERE id = ?`, id,
	).Scan(&d.ID, &d.Designation, &d.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// PostTransmission creates a new transmission for the given drone.
func (s *Store) PostTransmission(droneID int64, body string) (*Transmission, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("transmission body is empty")
	}
	if len(body) > 500 {
		return nil, errors.New("transmission exceeds 500 characters")
	}
	res, err := s.db.Exec(
		`INSERT INTO transmissions(drone_id, body) VALUES (?, ?)`,
		droneID, body,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	var t Transmission
	err = s.db.QueryRow(
		`SELECT t.id, t.drone_id, d.designation, t.body, t.created_at
		   FROM transmissions t JOIN drones d ON d.id = t.drone_id
		  WHERE t.id = ?`, id,
	).Scan(&t.ID, &t.DroneID, &t.Designation, &t.Body, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTransmissions returns the most recent transmissions, newest first.
func (s *Store) ListTransmissions(limit int) ([]Transmission, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT t.id, t.drone_id, d.designation, t.body, t.created_at
		   FROM transmissions t JOIN drones d ON d.id = t.drone_id
		  ORDER BY t.created_at DESC, t.id DESC
		  LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Transmission
	for rows.Next() {
		var t Transmission
		if err := rows.Scan(&t.ID, &t.DroneID, &t.Designation, &t.Body, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListTransmissionsByDrone returns transmissions from a specific drone, newest first.
func (s *Store) ListTransmissionsByDrone(designation string, limit int) ([]Transmission, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT t.id, t.drone_id, d.designation, t.body, t.created_at
		   FROM transmissions t JOIN drones d ON d.id = t.drone_id
		  WHERE d.designation = ?
		  ORDER BY t.created_at DESC, t.id DESC
		  LIMIT ?`, designation, limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Transmission
	for rows.Next() {
		var t Transmission
		if err := rows.Scan(&t.ID, &t.DroneID, &t.Designation, &t.Body, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllDroneDesignations returns all unique drone designations that have made transmissions.
func (s *Store) ListAllDroneDesignations() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT d.designation 
		   FROM transmissions t JOIN drones d ON d.id = t.drone_id
		   ORDER BY d.designation`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var designations []string
	for rows.Next() {
		var designation string
		if err := rows.Scan(&designation); err != nil {
			return nil, err
		}
		designations = append(designations, designation)
	}
	return designations, rows.Err()
}
