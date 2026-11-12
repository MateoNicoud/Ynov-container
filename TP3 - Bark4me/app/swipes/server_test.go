package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/swipes", strings.NewReader(body)))
	return rec
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func matchOf(t *testing.T, rec *httptest.ResponseRecorder) bool {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("code %d : %s", rec.Code, rec.Body)
	}
	var r struct{ Match bool }
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	return r.Match
}

func TestTruffeDonneToujoursUnMatch(t *testing.T) {
	h := NewHandler(newMemStore())
	for _, dog := range []string{"chien-01", "chien-02", "chien-03", "chien-04", "chien-05"} {
		if !matchOf(t, post(t, h, `{"visitor":"v1","dog":"`+dog+`","action":"truffe"}`)) {
			t.Errorf("Truffe sans Match pour %s", dog)
		}
	}
}

func TestPanneauNeDonneJamaisDeMatch(t *testing.T) {
	h := NewHandler(newMemStore())
	for _, dog := range []string{"chien-01", "chien-02", "chien-03", "chien-04", "chien-05"} {
		if matchOf(t, post(t, h, `{"visitor":"v1","dog":"`+dog+`","action":"panneau"}`)) {
			t.Errorf("Panneau avec Match pour %s", dog)
		}
	}
}

func TestPatteEstDeterministe(t *testing.T) {
	s := Swipe{Visitor: "abc", Dog: "chien-04", Action: Patte}
	first := IsMatch(s)
	for i := 0; i < 20; i++ {
		if IsMatch(s) != first {
			t.Fatal("résultat différent pour le même Swipe")
		}
	}
	// Sur plusieurs Visiteurs, la Patte donne des Match et des non-Match.
	var yes, no int
	for _, v := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		if IsMatch(Swipe{Visitor: v, Dog: "chien-04", Action: Patte}) {
			yes++
		} else {
			no++
		}
	}
	if yes == 0 || no == 0 {
		t.Errorf("règle dégénérée : %d Match, %d sans Match", yes, no)
	}
}

func TestSwipeDejaJuge(t *testing.T) {
	h := NewHandler(newMemStore())
	body := `{"visitor":"v1","dog":"chien-01","action":"patte"}`
	post(t, h, body)
	rec := post(t, h, body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code %d, attendu 409", rec.Code)
	}
}

func TestErreursDeValidation(t *testing.T) {
	h := NewHandler(newMemStore())
	for name, body := range map[string]string{
		"json cassé":      `{`,
		"visiteur absent": `{"dog":"chien-01","action":"patte"}`,
		"chien absent":    `{"visitor":"v1","action":"patte"}`,
		"action inconnue": `{"visitor":"v1","dog":"chien-01","action":"coeur"}`,
	} {
		rec := post(t, h, body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s : code %d, attendu 400", name, rec.Code)
		}
		var e map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if e["error"] == "" {
			t.Errorf("%s : pas de champ error", name)
		}
	}
}

func TestProbes(t *testing.T) {
	s := newMemStore()
	h := NewHandler(s)
	if get(t, h, "/healthz").Code != http.StatusOK || get(t, h, "/readyz").Code != http.StatusOK {
		t.Fatal("probes en échec alors que tout va bien")
	}
	s.down = true
	if get(t, h, "/readyz").Code != http.StatusServiceUnavailable {
		t.Fatal("/readyz doit échouer quand Postgres est coupé")
	}
	if get(t, h, "/healthz").Code != http.StatusOK {
		t.Fatal("/healthz ne dépend pas de Postgres")
	}
}

func TestStatistiquesGlobales(t *testing.T) {
	h := NewHandler(newMemStore())
	post(t, h, `{"visitor":"v1","dog":"chien-01","action":"patte"}`)
	post(t, h, `{"visitor":"v1","dog":"chien-02","action":"patte"}`)
	post(t, h, `{"visitor":"v2","dog":"chien-01","action":"truffe"}`)
	post(t, h, `{"visitor":"v2","dog":"chien-02","action":"panneau"}`)
	post(t, h, `{"visitor":"v2","dog":"chien-02","action":"panneau"}`) // doublon ignoré
	rec := get(t, h, "/api/swipes/stats")
	var st Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st != (Stats{Patte: 2, Truffe: 1, Panneau: 1}) {
		t.Errorf("totaux inattendus : %+v", st)
	}
}
