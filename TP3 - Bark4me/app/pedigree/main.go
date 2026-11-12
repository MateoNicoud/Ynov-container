package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	db, err := sql.Open("pgx", env("DATABASE_URL", "postgres://bark4me:bark4me@postgres:5432/bark4me"))
	if err != nil {
		log.Fatal(err)
	}
	var store *pgStore
	// Postgres peut démarrer après le service : on réessaie un moment.
	for i := 0; i < 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		store, err = newPGStore(ctx, db)
		cancel()
		if err == nil {
			break
		}
		log.Printf("postgres pas prêt : %v", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	addr := ":" + env("PORT", "8080")
	log.Printf("pedigree écoute sur %s", addr)
	log.Fatal(http.ListenAndServe(addr, NewHandler(store)))
}
