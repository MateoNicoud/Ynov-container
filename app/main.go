// API de démonstration pour les TP 3 à 9.
//
// Elle lit les articles publiés de WordPress directement dans MariaDB et expose
// de quoi pratiquer les sondes Kubernetes (/healthz, /readyz) et l'autoscaling (/burn).
// Les identifiants se lisent dans des fichiers (DB_USER_FILE, DB_PASSWORD_FILE),
// relus à chaque requête : ça marche avec les identifiants dynamiques d'OpenBao.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Remplacé au build : go build -ldflags "-X main.version=1.2.3"
var version = "dev"

// secret lit NAME_FILE s'il est défini, sinon NAME.
// Même convention que les images officielles mariadb et wordpress.
func secret(name string) (string, error) {
	if path := os.Getenv(name + "_FILE"); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	return os.Getenv(name), nil
}

func getenv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// store garde une connexion MariaDB et la rouvre quand les identifiants changent.
type store struct {
	mu   sync.Mutex
	dsn  string
	db   *sql.DB
	host string
	name string
	tls  bool
}

func (s *store) get() (*sql.DB, error) {
	user, err := secret("DB_USER")
	if err != nil {
		return nil, err
	}
	pass, err := secret("DB_PASSWORD")
	if err != nil {
		return nil, err
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = user, pass
	cfg.Net, cfg.Addr, cfg.DBName = "tcp", s.host, s.name
	cfg.Timeout = 2 * time.Second
	if s.tls {
		cfg.TLSConfig = "true" // obligatoire sur Azure Database for MySQL (TP 9)
	}
	dsn := cfg.FormatDSN()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil && dsn == s.dsn {
		return s.db, nil
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxLifetime(time.Minute)
	if s.db != nil {
		s.db.Close()
		slog.Info("identifiants changés, nouvelle connexion", "user", user)
	}
	s.db, s.dsn = db, dsn
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

// burnDuration borne le paramètre ms de /burn entre 0 et 1000.
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

func newMux(s *store, prefix string) *http.ServeMux {
	hostname, _ := os.Hostname()
	mux := http.NewServeMux()

	// Liveness : le processus répond. Ne touche pas à la base, exprès.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})

	// Readiness : la base répond. Sinon le Pod sort du Service.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		db, err := s.get()
		if err == nil {
			err = db.PingContext(ctx)
		}
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "ko", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": version, "hostname": hostname})
	})

	mux.HandleFunc("GET /api/posts", func(w http.ResponseWriter, r *http.Request) {
		db, err := s.get()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		rows, err := db.QueryContext(r.Context(), fmt.Sprintf(
			"SELECT ID, post_title, post_date FROM %sposts WHERE post_status = 'publish' AND post_type = 'post' ORDER BY post_date DESC LIMIT 10", prefix))
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

	// Consomme du CPU pendant ?ms= millisecondes (100 par défaut, 1000 max), pour le TP HPA.
	mux.HandleFunc("GET /burn", func(w http.ResponseWriter, r *http.Request) {
		d := burnDuration(r.URL.Query().Get("ms"))
		end := time.Now().Add(d)
		n := 0
		for time.Now().Before(end) {
			n++
		}
		writeJSON(w, http.StatusOK, map[string]any{"burned_ms": d.Milliseconds(), "hostname": hostname})
	})

	return mux
}

// healthcheck sert au HEALTHCHECK Docker : l'image distroless n'a ni shell ni curl.
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

	s := &store{
		host: getenv("DB_HOST", "mariadb:3306"),
		name: getenv("DB_NAME", "wordpress"),
		tls:  getenv("DB_TLS", "false") == "true",
	}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           newMux(s, getenv("DB_TABLE_PREFIX", "wp_")),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Kubernetes envoie SIGTERM avant de tuer le Pod : on finit les requêtes en cours.
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
}
