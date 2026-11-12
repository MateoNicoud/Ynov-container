package main

import (
	"log"
	"net/http"
	"os"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Usage : `photos` sert le volume ; `photos init` y copie les photos livrées
// dans l'image (conteneur d'initialisation).
func main() {
	data := env("PHOTOS_DIR", "/data/photos")
	if len(os.Args) > 1 && os.Args[1] == "init" {
		n, err := InitVolume(env("SEED_DIR", "/seed"), data)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("init : %d photo(s) copiée(s) vers %s", n, data)
		return
	}
	addr := ":" + env("PORT", "8080")
	log.Printf("photos écoute sur %s (volume %s)", addr, data)
	log.Fatal(http.ListenAndServe(addr, NewHandler(data)))
}
