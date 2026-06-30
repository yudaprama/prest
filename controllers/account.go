package controllers

import (
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/prest/prest/v2/adapters/postgres"
)

// Account self-service closure. Lives here (same package as workspaces.go) so it
// reuses extractUserID / ketoClient / deleteAllTuplesForObject / deleteSubjectTuples
// / ListWorkspacesForUser. Cookie-authed via the Oathkeeper edge
// (prest-workspaces-v1 rule, widened to /v1/account.*), which injects X-User-Id
// authoritatively — so a caller can only ever close their OWN account. The
// request body is intentionally empty; the user id comes from the edge, never
// the client.

// AccountDeleteHandler: POST /v1/account/delete — irreversibly closes the
// caller's account. Purges, in order:
//  1. Kawai content by user_id (personal-scope rows + their shared-workspace
//     rows). Leaf tables first; sessions cascades its messages/topics.
//  2. Workspaces owned by the caller (FK cascade clears any remaining members'
//     content in those workspaces — same semantics as DELETE /v1/workspaces).
//  3. workspace_members rows (membership in workspaces the caller did not own).
//  4. Keto tuples: wipe every tuple on owned workspaces + remove the subject
//     from every workspace they belonged to.
//  5. Kratos identity (loopback admin :4434) — LAST. Idempotent (404 = already
//     gone) so a retry after a partial failure is safe; deleting the identity
//     also kills all Kratos sessions server-side, invalidating the cookie.
//
// Talos API keys are NOT revoked here (separate service): the frontend revokes
// every own key first via /v2alpha1/self/issuedApiKeys/:revoke before calling
// this. Any orphaned key row is unusable once the Kratos identity is gone
// (ext_authz → Talos verify resolves the actor against Kratos).
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
	db, err := postgres.Get()
	if err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not configured")
		return
	}
	kc := ketoClient()
	ctx := r.Context()

	// Reverse-lookup the user's workspace set BEFORE deleting anything, so Keto
	// tuple cleanup still has the object list once the rows are gone.
	var wsIDs []string
	if kc.Enabled() {
		wsIDs, _ = kc.ListWorkspacesForUser(ctx, userID)
	}

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

	// 2) Owned workspaces. RETURNING the ids so we can wipe their Keto tuples;
	// the row delete FK-cascades workspace_members + any leftover scoped content.
	ownedRows, err := db.QueryContext(ctx,
		`DELETE FROM workspaces WHERE owner_id = $1 RETURNING id`, userID)
	if err != nil {
		slog.Error("account delete: delete owned workspaces", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not purge account data")
		return
	}
	var ownedIDs []string
	for ownedRows.Next() {
		var id string
		_ = ownedRows.Scan(&id)
		ownedIDs = append(ownedIDs, id)
	}
	ownedRows.Close()

	// 3) Membership rows in workspaces the caller did NOT own.
	if _, err := db.ExecContext(ctx,
		`DELETE FROM workspace_members WHERE user_id = $1`, userID); err != nil {
		slog.Error("account delete: delete memberships", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not purge account data")
		return
	}

	// 4) Keto cleanup. Best-effort per tuple (logged, non-fatal) so a Keto hiccup
	// never blocks a closure that has already purged Postgres + the identity.
	if kc.Enabled() {
		for _, id := range ownedIDs {
			if err := deleteAllTuplesForObject(ctx, kc, id); err != nil {
				slog.Error("account delete: keto object cleanup", "workspace", id, "err", err)
			}
		}
		for _, id := range wsIDs {
			if err := deleteSubjectTuples(ctx, kc, id, userID); err != nil {
				slog.Error("account delete: keto subject cleanup", "workspace", id, "err", err)
			}
		}
	}

	// 5) Kratos identity (loopback admin). Done last; idempotent. The base URL is
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		slog.Error("account delete: kratos identity delete status", "user", userID, "status", resp.StatusCode)
		writeJSONError(w, http.StatusBadGateway, "identity cleanup failed")
		return
	}

	slog.Info("account deleted", "user", userID,
		"owned", len(ownedIDs), "member_of", len(wsIDs))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
