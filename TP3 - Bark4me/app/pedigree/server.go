package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"net/http"
	"regexp"
	"strings"
	"time"
)

//go:embed certificate.svg
var certificateTemplate string

// Cert est un Pédigrée délivré à un chien.
type Cert struct {
	Dog      string    `json:"dog"`
	Name     string    `json:"name"`
	Breed    string    `json:"breed"`
	Number   string    `json:"number"`
	IssuedAt time.Time `json:"issued_at"`
}

// ErrNotFound : aucun Pédigrée pour ce numéro.
var ErrNotFound = errors.New("pédigrée inconnu")

// Store est le stockage des Pédigrées (Postgres en production, mémoire en test).
type Store interface {
	// Issue délivre un Pédigrée au chien, ou renvoie celui qu'il a déjà.
	Issue(ctx context.Context, c Cert) (Cert, error)
	ByNumber(ctx context.Context, number string) (Cert, error)
	List(ctx context.Context) ([]Cert, error)
	Ping(ctx context.Context) error
}

var (
	numberPattern = regexp.MustCompile(`^BARK-[0-9]{4}$`)
	idPattern     = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
)

// NumberFor propose un numéro BARK-XXXX stable pour un chien ; attempt décale le
// numéro en cas de collision avec un autre chien.
func NumberFor(dog string, attempt int) string {
	h := fnv.New32a()
	h.Write([]byte(dog))
	return fmt.Sprintf("BARK-%04d", (int(h.Sum32()%10000)+attempt)%10000)
}

// Certificate rend le certificat en SVG.
func Certificate(c Cert) string {
	return strings.NewReplacer(
		"{{NOM}}", html.EscapeString(c.Name),
		"{{RACE}}", html.EscapeString(c.Breed),
		"{{NUMERO}}", html.EscapeString(c.Number),
		"{{DATE}}", c.IssuedAt.Format("02/01/2006"),
	).Replace(certificateTemplate)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// Issue délivre un Pédigrée en cherchant un numéro libre.
func Issue(ctx context.Context, st Store, dog, name, breed string) (Cert, error) {
	for attempt := 0; attempt < 10000; attempt++ {
		c, err := st.Issue(ctx, Cert{Dog: dog, Name: name, Breed: breed, Number: NumberFor(dog, attempt), IssuedAt: time.Now().UTC()})
		if !errors.Is(err, errNumberTaken) {
			return c, err
		}
	}
	return Cert{}, errors.New("plus de numéro libre")
}

var errNumberTaken = errors.New("numéro déjà pris par un autre chien")

// NewHandler construit le routeur HTTP du service pedigree.
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
	mux.HandleFunc("GET /api/pedigree", func(w http.ResponseWriter, r *http.Request) {
		cs, err := st.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "lecture des Pédigrées impossible")
			return
		}
		writeJSON(w, http.StatusOK, cs)
	})
	mux.HandleFunc("POST /api/pedigree", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Dog, Name, Breed string }
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10))
		if err := dec.Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "corps JSON invalide")
			return
		}
		if !idPattern.MatchString(in.Dog) || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Breed) == "" {
			writeError(w, http.StatusBadRequest, "dog, name et breed sont obligatoires")
			return
		}
		c, err := Issue(r.Context(), st, in.Dog, in.Name, in.Breed)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "délivrance impossible")
			return
		}
		writeJSON(w, http.StatusCreated, c)
	})
	lookup := func(w http.ResponseWriter, r *http.Request) (Cert, bool) {
		n := r.PathValue("number")
		if !numberPattern.MatchString(n) {
			writeError(w, http.StatusBadRequest, "numéro invalide : attendu BARK-XXXX")
			return Cert{}, false
		}
		c, err := st.ByNumber(r.Context(), n)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "aucun Pédigrée avec ce numéro")
			return Cert{}, false
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "vérification impossible")
			return Cert{}, false
		}
		return c, true
	}
	mux.HandleFunc("GET /api/pedigree/{number}", func(w http.ResponseWriter, r *http.Request) {
		if c, ok := lookup(w, r); ok {
			writeJSON(w, http.StatusOK, map[string]any{"valid": true, "certificate": c})
		}
	})
	mux.HandleFunc("GET /api/pedigree/{number}/certificate.svg", func(w http.ResponseWriter, r *http.Request) {
		if c, ok := lookup(w, r); ok {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(Certificate(c)))
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "route inconnue")
	})
	return mux
}
