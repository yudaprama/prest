package controllers

import (
	"io"
	"log/slog"
	"net/http"
	"os"
)

// Account self-service closure. Cookie-authed via the Oathkeeper edge,
// which injects X-User-Id authoritatively — so a caller can only ever close
// their OWN account. The request body is intentionally empty; the user id
// comes from the edge, never the client.

// AccountDeleteHandler: POST /v1/account/delete — irreversibly closes the
// caller's account. Purges, in order:
//  1. Kawai content by user_id (personal-scope rows + tenant-scoped rows).
//     Leaf tables first; sessions cascades its messages/topics.
//  2. Kratos identity (loopback admin :4434) — LAST. Idempotent (404 = already
//     gone) so a retry after a partial failure is safe; deleting the identity
//     also kills all Kratos sessions server-side, invalidating the cookie.
//
// Talos API keys are NOT revoked here (separate service): the frontend revokes
// every own key first via /v2alpha1/self/issuedApiKeys/:revoke before calling
// this. Any orphaned key row is unusable once the Kratos identity is gone
// (ext_authz → Talos verify resolves the actor against Kratos).
//
// Tenant/membership cleanup is handled by the frontend before this call:
//   - For owned tenants: DELETE /v1/workspaces/{id}
//   - Leaving other tenants: DELETE /v1/workspaces/{id}/members/{m}
func AccountDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := extractUserID(r)
	if userID == "" || userID == "anonymous" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	db, err := kawaiDB()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	ctx := r.Context()

	// 1) Content by user_id. Order matters for FK safety: messages/topics reference
	// sessions (cascade), so delete the children first; sessions last.
	contentTables := []string{
		"agents",
		"document_chunks", "documents",
		"embeddings", "file_chunks", "chunks", "files",
		"messages", "topics",
		"sessions",
	}
	for _, t := range contentTables {
		if _, err := db.ExecContext(ctx, `DELETE FROM `+t+` WHERE user_id = $1`, userID); err != nil {
			slog.Error("account delete: purge table", "table", t, "user", userID, "err", err)
			writeJSONError(w, http.StatusBadGateway, "could not purge account data")
			return
		}
	}

	// 2) Kratos identity (loopback admin). Done last; idempotent. The base URL is
	// loopback-only and relies on network isolation (no token), matching how the
	// bootstrap webhook reaches pREST.
	kratosAdmin := os.Getenv("KRATOS_ADMIN_URL")
	if kratosAdmin == "" {
		kratosAdmin = "http://localhost:4434"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		kratosAdmin+"/admin/identities/"+userID, nil)
	if err != nil {
		slog.Error("account delete: build kratos request", "err", err)
		writeJSONError(w, http.StatusBadGateway, "identity cleanup failed")
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("account delete: kratos identity delete", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "identity cleanup failed")
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		slog.Error("account delete: kratos identity delete status", "user", userID, "status", resp.StatusCode)
		writeJSONError(w, http.StatusBadGateway, "identity cleanup failed")
		return
	}

	slog.Info("account deleted", "user", userID)
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
