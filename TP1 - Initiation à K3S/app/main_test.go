package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBurnDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"":     100 * time.Millisecond,
		"abc":  100 * time.Millisecond,
		"-5":   100 * time.Millisecond,
		"250":  250 * time.Millisecond,
		"5000": time.Second,
	}
	for in, want := range cases {
		if got := burnDuration(in); got != want {
			t.Errorf("burnDuration(%q) = %v ; attendu %v", in, got, want)
		}
	}
}

func TestHealthzDoesNotNeedDatabase(t *testing.T) {
	t.Setenv("DB_HOST", "nulle-part:3306")
	db, err := openDB()
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	newMux(db).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz = %d ; attendu 200", rec.Code)
	}
}
