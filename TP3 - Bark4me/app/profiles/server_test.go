package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestListeLesDixProfils(t *testing.T) {
	rec := get(t, NewHandler(newMemStore()), "/api/profiles")
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	var ps []Profile
	if err := json.Unmarshal(rec.Body.Bytes(), &ps); err != nil {
		t.Fatal(err)
	}
	if len(ps) != 10 {
		t.Fatalf("%d Profils, attendu 10", len(ps))
	}
	for _, p := range ps {
		if p.ID == "" || p.Name == "" || p.Breed == "" || p.Tagline == "" || p.Photo == "" {
			t.Errorf("Profil incomplet : %+v", p)
		}
	}
	if ps[0].Name != "Jean Baguette" || ps[0].Photo != "/photos/chien-01.jpg" {
		t.Errorf("premier Profil inattendu : %+v", ps[0])
	}
}

func TestHealthz(t *testing.T) {
	if rec := get(t, NewHandler(newMemStore()), "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
}

func TestReadyzSuitPostgres(t *testing.T) {
	s := newMemStore()
	h := NewHandler(s)
	if rec := get(t, h, "/readyz"); rec.Code != http.StatusOK {
		t.Fatalf("prêt : code %d", rec.Code)
	}
	s.down = true
	rec := get(t, h, "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Postgres coupé : code %d", rec.Code)
	}
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] == "" {
		t.Errorf("corps sans champ error : %s", rec.Body)
	}
}

func TestRouteInconnueEnJSON(t *testing.T) {
	rec := get(t, NewHandler(newMemStore()), "/nimporte")
	if rec.Code != http.StatusNotFound || rec.Header().Get("Content-Type") == "" {
		t.Fatalf("code %d", rec.Code)
	}
}
