package controllers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	dbPool *pgxpool.Pool
	dbOnce sync.Once
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