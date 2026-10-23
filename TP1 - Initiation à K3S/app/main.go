package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
)

var version = "dev"

func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func openDB() (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD")
	cfg.Net, cfg.Addr, cfg.DBName = "tcp", getenv("DB_HOST", "mariadb:3306"), getenv("DB_NAME", "wordpress")
	cfg.Timeout = 2 * time.Second
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(time.Minute)
	return db, nil
}

type post struct {
	ID    int64     `json:"id"`
	Title string    `json:"title"`
	Date  time.Time `json:"date"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func burnDuration(raw string) time.Duration {
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 0 {
		ms = 100
	}
	if ms > 1000 {
		ms = 1000
	}
	return time.Duration(ms) * time.Millisecond
}

func newMux(db *sql.DB) *http.ServeMux {
	hostname, _ := os.Hostname()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "ko", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": version, "hostname": hostname})
	})

	mux.HandleFunc("GET /api/posts", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			"SELECT ID, post_title, post_date FROM wp_posts WHERE post_status = 'publish' AND post_type = 'post' ORDER BY post_date DESC LIMIT 10")
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		defer rows.Close()
		posts := []post{}
		for rows.Next() {
			var p post
			var raw []byte
			if err := rows.Scan(&p.ID, &p.Title, &raw); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			p.Date, _ = time.Parse(time.DateTime, string(raw))
			posts = append(posts, p)
		}
		writeJSON(w, http.StatusOK, posts)
	})

	mux.HandleFunc("GET /burn", func(w http.ResponseWriter, r *http.Request) {
		d := burnDuration(r.URL.Query().Get("ms"))
		end := time.Now().Add(d)
		for time.Now().Before(end) {
		}
		writeJSON(w, http.StatusOK, map[string]any{"burned_ms": d.Milliseconds(), "hostname": hostname})
	})

	return mux
}

func healthcheck(port string) int {
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	port := getenv("PORT", "8080")

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(port))
	}

	db, err := openDB()
	if err != nil {
		slog.Error("base", "error", err)
		os.Exit(1)
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newMux(db),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	go func() {
		slog.Info("démarrage", "version", version, "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serveur arrêté", "error", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	slog.Info("SIGTERM reçu, arrêt propre")
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutdown)
	db.Close()
}
