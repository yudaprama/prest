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

	// Gather the caller's workspace memberships from the membership mirror
	// (workspace_members), kept in sync with Keto on every create/member/leave
	// op. We use the DB rather than Keto ListWorkspacesForUser so this works even
	// when Keto is disabled — and, importantly, we do NOT read workspaces.owner_id,
	// which may be absent on older migrated schemas; the role on
	// workspace_members is the ownership source of truth.
	memberRows, err := db.QueryContext(ctx,
		`SELECT workspace_id, role FROM workspace_members WHERE user_id = $1`, userID)
	if err != nil {
		slog.Error("account delete: list memberships", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not purge account data")
		return
	}
	var ownedIDs, allIDs []string
	for memberRows.Next() {
		var id, role string
		if err := memberRows.Scan(&id, &role); err != nil {
			memberRows.Close()
			slog.Error("account delete: scan membership", "err", err)
			writeJSONError(w, http.StatusBadGateway, "could not purge account data")
			return
		}
		allIDs = append(allIDs, id)
		if role == roleOwner {
			ownedIDs = append(ownedIDs, id)
		}
	}
	memberRows.Close()

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

	// 2) Owned workspaces. Deleting the row FK-cascades its workspace_members +
	// any leftover scoped content (same semantics as DELETE /v1/workspaces).
	for _, id := range ownedIDs {
		if _, err := db.ExecContext(ctx, `DELETE FROM workspaces WHERE id = $1`, id); err != nil {
			slog.Error("account delete: delete owned workspace", "workspace", id, "user", userID, "err", err)
			writeJSONError(w, http.StatusBadGateway, "could not purge account data")
			return
		}
	}

	// 3) Remaining membership rows in workspaces the caller did NOT own.
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
		for _, id := range allIDs {
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
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		slog.Error("account delete: kratos identity delete status", "user", userID, "status", resp.StatusCode)
		writeJSONError(w, http.StatusBadGateway, "identity cleanup failed")
		return
	}

	slog.Info("account deleted", "user", userID,
		"owned", len(ownedIDs), "member_of", len(allIDs))
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
