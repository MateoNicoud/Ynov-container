package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSecretPrefersFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pw")
	os.WriteFile(path, []byte("depuis-fichier\n"), 0o600)
	t.Setenv("DB_PASSWORD", "depuis-env")
	t.Setenv("DB_PASSWORD_FILE", path)

	got, err := secret("DB_PASSWORD")
	if err != nil || got != "depuis-fichier" {
		t.Fatalf("secret() = %q, %v ; attendu depuis-fichier", got, err)
	}
}

func TestSecretFallsBackToEnv(t *testing.T) {
	t.Setenv("DB_PASSWORD", "depuis-env")
	got, _ := secret("DB_PASSWORD")
	if got != "depuis-env" {
		t.Fatalf("secret() = %q ; attendu depuis-env", got)
	}
}

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
	mux := newMux(&store{host: "nulle-part:3306", name: "x"}, "wp_")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz = %d ; attendu 200", rec.Code)
	}
}
