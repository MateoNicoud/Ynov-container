package main

import (
	"context"
	"database/sql"
	"errors"
)

// memStore garde les Profils en mémoire (tests).
type memStore struct {
	profiles []Profile
	down     bool
}

func newMemStore() *memStore { return &memStore{profiles: initialProfiles} }

func (m *memStore) List(context.Context) ([]Profile, error) { return m.profiles, nil }
func (m *memStore) Ping(context.Context) error {
	if m.down {
		return errors.New("down")
	}
	return nil
}

// pgStore garde les Profils dans Postgres.
type pgStore struct{ db *sql.DB }

// newPGStore crée la table et charge les Profils initiaux.
func newPGStore(ctx context.Context, db *sql.DB) (*pgStore, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS profiles (
		id text PRIMARY KEY, position int NOT NULL, name text NOT NULL,
		breed text NOT NULL, tagline text NOT NULL, photo text NOT NULL)`); err != nil {
		return nil, err
	}
	for i, p := range initialProfiles {
		if _, err := db.ExecContext(ctx, `INSERT INTO profiles (id, position, name, breed, tagline, photo)
			VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (id) DO NOTHING`,
			p.ID, i, p.Name, p.Breed, p.Tagline, p.Photo); err != nil {
			return nil, err
		}
	}
	return &pgStore{db: db}, nil
}

func (s *pgStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *pgStore) List(ctx context.Context) ([]Profile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, breed, tagline, photo FROM profiles ORDER BY position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Profile{}
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.Breed, &p.Tagline, &p.Photo); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
