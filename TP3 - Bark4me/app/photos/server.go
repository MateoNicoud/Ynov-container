package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// marker est créé quand les photos livrées ont été copiées dans le volume.
const marker = ".initialized"

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// NewHandler sert les images du dossier dir sous /photos/.
func NewHandler(dir string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"status":"ok"}`+"\n")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			writeError(w, http.StatusServiceUnavailable, "volume de photos indisponible")
			return
		}
		if _, err := os.Stat(filepath.Join(dir, marker)); err != nil {
			writeError(w, http.StatusServiceUnavailable, "photos pas encore initialisées")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"status":"ready"}`+"\n")
	})
	mux.HandleFunc("GET /photos/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !namePattern.MatchString(name) {
			writeError(w, http.StatusNotFound, "photo inconnue")
			return
		}
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err != nil || fi.IsDir() {
			writeError(w, http.StatusNotFound, "photo inconnue")
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.ServeFile(w, r, p)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "route inconnue")
	})
	return mux
}

// InitVolume copie les photos livrées (src) vers le volume partagé (dst).
// Il ne fait rien si le volume a déjà été initialisé. Renvoie le nombre de
// fichiers copiés.
func InitVolume(src, dst string) (int, error) {
	if _, err := os.Stat(filepath.Join(dst, marker)); err == nil {
		return 0, nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return n, err
		}
		n++
	}
	return n, os.WriteFile(filepath.Join(dst, marker), nil, 0o644)
}

// copyFile copie via un fichier temporaire puis renomme, pour ne jamais
// exposer une image à moitié écrite aux autres instances.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".copy-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}
