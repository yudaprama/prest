package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
)

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

func kawaiDB() (*sql.DB, error) {
	dsn := os.Getenv("KAWAI_PG_DSN")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	return sql.Open("postgres", dsn)
}
