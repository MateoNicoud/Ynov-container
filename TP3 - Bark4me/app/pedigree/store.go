package main

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"sync"
)

// Pédigrées délivrés au démarrage aux chiens « de race » du Paquet.
var seedCerts = []Cert{
	{Dog: "chien-01", Name: "Jean Baguette", Breed: "Bouledogue français"},
	{Dog: "chien-03", Name: "Mage Gérard", Breed: "Carlin"},
	{Dog: "chien-07", Name: "Rex", Breed: "Chien (vérifié)"},
}

// memStore garde les Pédigrées en mémoire (tests).
type memStore struct {
	mu   sync.Mutex
	byID map[string]Cert
	down bool
}

func newMemStore() *memStore { return &memStore{byID: map[string]Cert{}} }

func (m *memStore) Issue(_ context.Context, c Cert) (Cert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if old, ok := m.byID[c.Dog]; ok {
		return old, nil
	}
	for _, o := range m.byID {
		if o.Number == c.Number {
			return Cert{}, errNumberTaken
		}
	}
	m.byID[c.Dog] = c
	return c, nil
}

func (m *memStore) ByNumber(_ context.Context, n string) (Cert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.byID {
		if c.Number == n {
			return c, nil
		}
	}
	return Cert{}, ErrNotFound
}

func (m *memStore) List(context.Context) ([]Cert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Cert{}
	for _, c := range m.byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dog < out[j].Dog })
	return out, nil
}

func (m *memStore) Ping(context.Context) error {
	if m.down {
		return errors.New("down")
	}
	return nil
}

// pgStore garde les Pédigrées dans Postgres.
type pgStore struct{ db *sql.DB }

func newPGStore(ctx context.Context, db *sql.DB) (*pgStore, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS pedigrees (
		dog text PRIMARY KEY, name text NOT NULL, breed text NOT NULL,
		number text NOT NULL UNIQUE, issued_at timestamptz NOT NULL)`); err != nil {
		return nil, err
	}
	s := &pgStore{db: db}
	for _, c := range seedCerts {
		if _, err := Issue(ctx, s, c.Dog, c.Name, c.Breed); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *pgStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *pgStore) Issue(ctx context.Context, c Cert) (Cert, error) {
	// Le chien a déjà un Pédigrée : on le renvoie.
	var old Cert
	err := s.db.QueryRowContext(ctx, `SELECT dog, name, breed, number, issued_at FROM pedigrees WHERE dog=$1`, c.Dog).
		Scan(&old.Dog, &old.Name, &old.Breed, &old.Number, &old.IssuedAt)
	if err == nil {
		return old, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Cert{}, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO pedigrees (dog, name, breed, number, issued_at)
		VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, c.Dog, c.Name, c.Breed, c.Number, c.IssuedAt)
	if err != nil {
		return Cert{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Conflit : un autre chien a ce numéro, ou le chien vient d'être inséré en parallèle.
		if err := s.db.QueryRowContext(ctx, `SELECT dog, name, breed, number, issued_at FROM pedigrees WHERE dog=$1`, c.Dog).
			Scan(&old.Dog, &old.Name, &old.Breed, &old.Number, &old.IssuedAt); err == nil {
			return old, nil
		}
		return Cert{}, errNumberTaken
	}
	return c, nil
}

func (s *pgStore) ByNumber(ctx context.Context, n string) (Cert, error) {
	var c Cert
	err := s.db.QueryRowContext(ctx, `SELECT dog, name, breed, number, issued_at FROM pedigrees WHERE number=$1`, n).
		Scan(&c.Dog, &c.Name, &c.Breed, &c.Number, &c.IssuedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Cert{}, ErrNotFound
	}
	return c, err
}

func (s *pgStore) List(ctx context.Context) ([]Cert, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT dog, name, breed, number, issued_at FROM pedigrees ORDER BY dog`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Cert{}
	for rows.Next() {
		var c Cert
		if err := rows.Scan(&c.Dog, &c.Name, &c.Breed, &c.Number, &c.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
