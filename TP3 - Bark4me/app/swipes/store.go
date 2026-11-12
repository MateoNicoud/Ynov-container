package main

import (
	"context"
	"database/sql"
	"errors"
	"sync"
)

// memStore garde les Swipes en mémoire (tests).
type memStore struct {
	mu    sync.Mutex
	seen  map[[2]string]string
	stats Stats
	down  bool
}

func newMemStore() *memStore { return &memStore{seen: map[[2]string]string{}} }

func (m *memStore) Record(_ context.Context, s Swipe) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := [2]string{s.Visitor, s.Dog}
	if _, ok := m.seen[k]; ok {
		return ErrDuplicate
	}
	m.seen[k] = s.Action
	switch s.Action {
	case Patte:
		m.stats.Patte++
	case Truffe:
		m.stats.Truffe++
	case Panneau:
		m.stats.Panneau++
	}
	return nil
}

func (m *memStore) Stats(context.Context) (Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats, nil
}

func (m *memStore) Ping(context.Context) error {
	if m.down {
		return errors.New("down")
	}
	return nil
}

// pgStore garde les Swipes dans Postgres.
type pgStore struct{ db *sql.DB }

func newPGStore(ctx context.Context, db *sql.DB) (*pgStore, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS swipes (
		visitor text NOT NULL, dog text NOT NULL, action text NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY (visitor, dog))`); err != nil {
		return nil, err
	}
	return &pgStore{db: db}, nil
}

func (s *pgStore) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *pgStore) Record(ctx context.Context, sw Swipe) error {
	res, err := s.db.ExecContext(ctx, `INSERT INTO swipes (visitor, dog, action) VALUES ($1,$2,$3)
		ON CONFLICT (visitor, dog) DO NOTHING`, sw.Visitor, sw.Dog, sw.Action)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrDuplicate
	}
	return nil
}

func (s *pgStore) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, `SELECT
		count(*) FILTER (WHERE action='patte'),
		count(*) FILTER (WHERE action='truffe'),
		count(*) FILTER (WHERE action='panneau') FROM swipes`).Scan(&st.Patte, &st.Truffe, &st.Panneau)
	return st, err
}
