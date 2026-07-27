package controllers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	dbPool    *pgxpool.Pool
	dbOnce    sync.Once
	talosPool *pgxpool.Pool
	talosOnce sync.Once
)

func kawaiDB() *pgxpool.Pool {
	dbOnce.Do(func() {
		dsn := os.Getenv("KAWAI_PG_DSN")
		if dsn == "" {
			dsn = os.Getenv("DATABASE_URL")
		}
		pool, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			panic("kawaiDB: " + err.Error())
		}
		dbPool = pool
	})
	return dbPool
}

// talosDB returns the Talos (API-key metering/quota) pool. Best-effort: if
// TALOS_DSN is unset the pool is nil and callers must nil-check before use.
func talosDB() *pgxpool.Pool {
	talosOnce.Do(func() {
		dsn := os.Getenv("TALOS_DSN")
		if dsn == "" {
			return
		}
		pool, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			slog.Error("talosDB: init pool", "err", err)
			return
		}
		talosPool = pool
	})
	return talosPool
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func extractUserID(r *http.Request) string {
	return r.Header.Get("X-User-Id")
}
