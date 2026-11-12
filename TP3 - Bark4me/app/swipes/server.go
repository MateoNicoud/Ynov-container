package main

import (
	"context"
	"encoding/json"
	"errors"
	"hash/fnv"
	"net/http"
	"regexp"
	"time"
)

// Actions possibles d'un Swipe.
const (
	Patte   = "patte"
	Truffe  = "truffe"
	Panneau = "panneau"
)

// Swipe est le choix d'un Visiteur sur un Profil.
type Swipe struct {
	Visitor string `json:"visitor"`
	Dog     string `json:"dog"`
	Action  string `json:"action"`
}

// Stats donne le nombre total de Pattes, Truffes et Panneaux.
type Stats struct {
	Patte   int `json:"patte"`
	Truffe  int `json:"truffe"`
	Panneau int `json:"panneau"`
}

// ErrDuplicate : ce Visiteur a déjà jugé ce Profil.
var ErrDuplicate = errors.New("profil déjà jugé")

// Store est le stockage des Swipes (Postgres en production, mémoire en test).
type Store interface {
	Record(ctx context.Context, s Swipe) error
	Stats(ctx context.Context) (Stats, error)
	Ping(ctx context.Context) error
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// IsMatch applique la règle de Match : Truffe toujours, Panneau jamais,
// Patte de façon déterministe à partir du chien et du Visiteur.
func IsMatch(s Swipe) bool {
	switch s.Action {
	case Truffe:
		return true
	case Patte:
		h := fnv.New32a()
		h.Write([]byte(s.Dog + "|" + s.Visitor))
		return h.Sum32()%2 == 0
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// NewHandler construit le routeur HTTP du service swipes.
func NewHandler(st Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := st.Ping(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "postgres indisponible")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /api/swipes", func(w http.ResponseWriter, r *http.Request) {
		var s Swipe
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
		if err := dec.Decode(&s); err != nil {
			writeError(w, http.StatusBadRequest, "corps JSON invalide")
			return
		}
		if !idPattern.MatchString(s.Visitor) {
			writeError(w, http.StatusBadRequest, "visitor invalide")
			return
		}
		if !idPattern.MatchString(s.Dog) {
			writeError(w, http.StatusBadRequest, "dog invalide")
			return
		}
		if s.Action != Patte && s.Action != Truffe && s.Action != Panneau {
			writeError(w, http.StatusBadRequest, "action invalide : patte, truffe ou panneau")
			return
		}
		if err := st.Record(r.Context(), s); err != nil {
			if errors.Is(err, ErrDuplicate) {
				writeError(w, http.StatusConflict, "ce Profil a déjà été jugé")
				return
			}
			writeError(w, http.StatusInternalServerError, "enregistrement impossible")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"dog": s.Dog, "action": s.Action, "match": IsMatch(s)})
	})
	mux.HandleFunc("GET /api/swipes/stats", func(w http.ResponseWriter, r *http.Request) {
		stats, err := st.Stats(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lecture des statistiques impossible")
			return
		}
		writeJSON(w, http.StatusOK, stats)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "route inconnue")
	})
	return mux
}
