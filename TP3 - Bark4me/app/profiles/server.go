package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Profile est la fiche d'un chien affichée sur une carte du Paquet.
type Profile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Breed   string `json:"breed"`
	Tagline string `json:"tagline"`
	Photo   string `json:"photo"`
}

// Store est le stockage des Profils (Postgres en production, mémoire en test).
type Store interface {
	List(ctx context.Context) ([]Profile, error)
	Ping(ctx context.Context) error
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// NewHandler construit le routeur HTTP du service profiles.
func NewHandler(s Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "postgres indisponible")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/profiles", func(w http.ResponseWriter, r *http.Request) {
		ps, err := s.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lecture des Profils impossible")
			return
		}
		writeJSON(w, http.StatusOK, ps)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "route inconnue")
	})
	return mux
}
