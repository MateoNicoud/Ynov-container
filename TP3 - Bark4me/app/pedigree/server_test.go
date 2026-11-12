package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func issue(t *testing.T, h http.Handler, dog string) Cert {
	t.Helper()
	rec := do(t, h, http.MethodPost, "/api/pedigree", `{"dog":"`+dog+`","name":"Rex","breed":"Carlin"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code %d : %s", rec.Code, rec.Body)
	}
	var c Cert
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDelivranceDunNumero(t *testing.T) {
	h := NewHandler(newMemStore())
	c := issue(t, h, "chien-01")
	if !regexp.MustCompile(`^BARK-[0-9]{4}$`).MatchString(c.Number) {
		t.Fatalf("numéro %q", c.Number)
	}
	if again := issue(t, h, "chien-01"); again.Number != c.Number {
		t.Errorf("deuxième délivrance : %s au lieu de %s", again.Number, c.Number)
	}
}

func TestVerificationNumeroConnu(t *testing.T) {
	h := NewHandler(newMemStore())
	c := issue(t, h, "chien-03")
	rec := do(t, h, http.MethodGet, "/api/pedigree/"+c.Number, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d", rec.Code)
	}
	var out struct {
		Valid       bool
		Certificate Cert
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.Valid || out.Certificate.Dog != "chien-03" {
		t.Errorf("réponse inattendue : %s", rec.Body)
	}
}

func TestNumeroInconnu(t *testing.T) {
	h := NewHandler(newMemStore())
	rec := do(t, h, http.MethodGet, "/api/pedigree/BARK-0000", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d, attendu 404", rec.Code)
	}
	var e map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &e)
	if e["error"] == "" {
		t.Error("pas de champ error")
	}
	if rec := do(t, h, http.MethodGet, "/api/pedigree/ABC", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("numéro mal formé : code %d, attendu 400", rec.Code)
	}
	if rec := do(t, h, http.MethodGet, "/api/pedigree/BARK-0000/certificate.svg", ""); rec.Code != http.StatusNotFound {
		t.Errorf("certificat inconnu : code %d", rec.Code)
	}
}

func TestCertificatSVG(t *testing.T) {
	h := NewHandler(newMemStore())
	rec := do(t, h, http.MethodPost, "/api/pedigree", `{"dog":"chien-05","name":"Tom & Jerry","breed":"Carlin"}`)
	var c Cert
	_ = json.Unmarshal(rec.Body.Bytes(), &c)
	rec = do(t, h, http.MethodGet, "/api/pedigree/"+c.Number+"/certificate.svg", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("code %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, c.Number) || !strings.Contains(body, "Tom &amp; Jerry") || strings.Contains(body, "{{") {
		t.Errorf("certificat mal rempli : %s", body)
	}
}

func TestCollisionDeNumeros(t *testing.T) {
	s := newMemStore()
	h := NewHandler(s)
	a := issue(t, h, "chien-01")
	// Un autre chien qui tomberait sur le même numéro en reçoit un différent.
	s.byID["autre"] = Cert{Dog: "autre", Number: NumberFor("chien-02", 0)}
	b := issue(t, h, "chien-02")
	if b.Number == s.byID["autre"].Number || b.Number == a.Number {
		t.Errorf("numéro en double : %s", b.Number)
	}
}

func TestListeEtErreurs(t *testing.T) {
	h := NewHandler(newMemStore())
	issue(t, h, "chien-07")
	var cs []Cert
	_ = json.Unmarshal(do(t, h, http.MethodGet, "/api/pedigree", "").Body.Bytes(), &cs)
	if len(cs) != 1 || cs[0].Dog != "chien-07" {
		t.Errorf("liste inattendue : %+v", cs)
	}
	if rec := do(t, h, http.MethodPost, "/api/pedigree", `{"dog":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("code %d, attendu 400", rec.Code)
	}
}

func TestProbes(t *testing.T) {
	s := newMemStore()
	h := NewHandler(s)
	if do(t, h, "GET", "/healthz", "").Code != 200 || do(t, h, "GET", "/readyz", "").Code != 200 {
		t.Fatal("probes en échec")
	}
	s.down = true
	if do(t, h, "GET", "/readyz", "").Code != http.StatusServiceUnavailable {
		t.Fatal("/readyz doit échouer sans Postgres")
	}
}
