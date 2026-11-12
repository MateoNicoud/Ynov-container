package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func seed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range []string{"chien-01.jpg", "chien-02.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("jpeg-"+n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestServeLesImagesDuVolume(t *testing.T) {
	src, dst := seed(t), filepath.Join(t.TempDir(), "photos")
	if _, err := InitVolume(src, dst); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(dst)
	rec := get(h, "/photos/chien-01.jpg")
	if rec.Code != http.StatusOK || rec.Body.String() != "jpeg-chien-01.jpg" {
		t.Fatalf("code %d, corps %q", rec.Code, rec.Body)
	}
	if rec := get(h, "/photos/absent.jpg"); rec.Code != http.StatusNotFound {
		t.Errorf("image absente : code %d", rec.Code)
	}
	if rec := get(h, "/photos/.initialized"); rec.Code != http.StatusNotFound {
		t.Errorf("le marqueur ne doit pas être servi : code %d", rec.Code)
	}
	if rec := get(h, "/photos/..%2Fsecret"); rec.Code != http.StatusNotFound {
		t.Errorf("traversée de dossier : code %d", rec.Code)
	}
}

func TestInitCopieAuPremierDemarrageSeulement(t *testing.T) {
	src, dst := seed(t), filepath.Join(t.TempDir(), "photos")
	n, err := InitVolume(src, dst)
	if err != nil || n != 2 {
		t.Fatalf("premier démarrage : %d copiées, err %v", n, err)
	}
	// Une photo modifiée dans le volume ne doit pas être écrasée au redémarrage.
	if err := os.WriteFile(filepath.Join(dst, "chien-01.jpg"), []byte("modifiée"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err = InitVolume(src, dst)
	if err != nil || n != 0 {
		t.Fatalf("second démarrage : %d copiées, err %v", n, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "chien-01.jpg")); string(b) != "modifiée" {
		t.Errorf("photo écrasée : %q", b)
	}
}

func TestPlusieursInstancesPartagentLeVolume(t *testing.T) {
	src, dst := seed(t), filepath.Join(t.TempDir(), "photos")
	if _, err := InitVolume(src, dst); err != nil {
		t.Fatal(err)
	}
	a, b := NewHandler(dst), NewHandler(dst)
	if get(a, "/photos/chien-02.jpg").Code != 200 || get(b, "/photos/chien-02.jpg").Code != 200 {
		t.Fatal("les deux instances doivent servir la même image")
	}
}

func TestProbes(t *testing.T) {
	src, dst := seed(t), filepath.Join(t.TempDir(), "photos")
	h := NewHandler(dst)
	if get(h, "/healthz").Code != http.StatusOK {
		t.Fatal("/healthz")
	}
	if get(h, "/readyz").Code != http.StatusServiceUnavailable {
		t.Fatal("/readyz doit échouer avant l'initialisation")
	}
	if _, err := InitVolume(src, dst); err != nil {
		t.Fatal(err)
	}
	if get(h, "/readyz").Code != http.StatusOK {
		t.Fatal("/readyz doit réussir après l'initialisation")
	}
}
