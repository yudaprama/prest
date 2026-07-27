package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
)

// Account self-service closure. Cookie-authed via the Oathkeeper edge,
// which injects X-User-Id authoritatively — so a caller can only ever close
// their OWN account. The request body is intentionally empty; the user id
// comes from the edge, never the client.

// AccountDeleteHandler: POST /v1/account/delete — irreversibly closes the
// caller's account. The user id comes from the edge (X-User-Id, authoritative,
// injected by Oathkeeper); the body is intentionally empty so a caller can
// only ever close their OWN account.
//
// Purge order:
//  1. Kawai content by user_id (one atomic transaction). Leaf tables first;
//     sessions/projects/scheduled_runs cascade their children. Also purges
//     user_connectors (+tools via cascade), user_installed_plugins, and the
//     workspace layer: tenants where the user is the SOLE member are deleted
//     (cascade clears their members + invites); remaining memberships are
//     removed (leave); invites created by or addressed to the user are dropped.
//  2. Talos cleanup (best-effort, separate pool, non-fatal): revokes the
//     user's issued/imported API keys, drops their balance and usage metering.
//     Talos is operational quota telemetry only — authoritative billing lives
//     in the separate yarsew DB, which is NOT touched here.
//  3. Kratos identity (loopback admin :4434) — LAST. Idempotent (404 = already
//     gone) so a retry after a partial failure is safe; deleting the identity
//     also kills all Kratos sessions server-side, invalidating the cookie.
//
// Out of scope (require separate work): physical file bytes in AList storage,
// shares table (no user_id column), agent_templates (global), river_* (job
// queue), MuninnDB engrams, yarsew billing audit.
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
	userEmail := strings.ToLower(strings.TrimSpace(r.Header.Get("X-User-Email")))
	db := kawaiDB()
	ctx := r.Context()

	// 1) Kawai purge — single transaction so the account closure is atomic
	// across content + connectors + workspaces. If anything fails, nothing is
	// committed and the identity (and thus login) stays intact for a retry.
	tx, err := db.Begin(ctx)
	if err != nil {
		slog.Error("account delete: begin tx", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not close account")
		return
	}
	defer tx.Rollback(ctx)

	// 1a) Content by user_id. Order matters for FK safety: messages/topics
	// reference sessions (cascade), project_members references projects
	// (cascade), scheduled_run_executions references scheduled_runs (cascade).
	// Children are deleted explicitly first, parents last — belt-and-suspenders
	// alongside the cascades.
	contentTables := []string{
		"agents",
		"document_chunks", "documents",
		"embeddings", "file_chunks", "chunks", "files",
		"messages", "topics",
		"sessions",
		"projects",       // cascade → project_members
		"scheduled_runs", // cascade → scheduled_run_executions
	}
	for _, t := range contentTables {
		if _, err := tx.Exec(ctx, `DELETE FROM `+t+` WHERE user_id = $1`, userID); err != nil {
			slog.Error("account delete: purge content", "table", t, "user", userID, "err", err)
			writeJSONError(w, http.StatusBadGateway, "could not purge account data")
			return
		}
	}

	// 1b) Connectors. user_connectors cascades user_connector_tools.
	if _, err := tx.Exec(ctx, `DELETE FROM user_connectors WHERE user_id = $1`, userID); err != nil {
		slog.Error("account delete: user_connectors", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not purge account data")
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_installed_plugins WHERE user_id = $1`, userID); err != nil {
		slog.Error("account delete: user_installed_plugins", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not purge account data")
		return
	}

	// 1c) Workspaces. Delete tenants where the user is the SOLE member (the
	// workspace dies with its only member); FK cascade clears their
	// tenant_members + tenant_invites. Surviving tenants keep their data; the
	// user is simply removed from them.
	if _, err := tx.Exec(ctx, `
		DELETE FROM tenants WHERE id IN (
			SELECT tenant_id FROM tenant_members
			WHERE user_id = $1
			GROUP BY tenant_id
			HAVING count(*) = 1
		)`, userID); err != nil {
		slog.Error("account delete: delete sole-member tenants", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not purge account data")
		return
	}

	// 1d) Invites the user created (invited_by) or that target their email
	// (can never be accepted once the identity is gone), on surviving tenants.
	if userEmail != "" {
		if _, err := tx.Exec(ctx, `DELETE FROM tenant_invites WHERE invited_by = $1 OR email = $2`, userID, userEmail); err != nil {
			slog.Error("account delete: tenant_invites", "user", userID, "err", err)
			writeJSONError(w, http.StatusBadGateway, "could not purge account data")
			return
		}
	} else {
		if _, err := tx.Exec(ctx, `DELETE FROM tenant_invites WHERE invited_by = $1`, userID); err != nil {
			slog.Error("account delete: tenant_invites", "user", userID, "err", err)
			writeJSONError(w, http.StatusBadGateway, "could not purge account data")
			return
		}
	}

	// 1e) Remaining memberships (leave workspaces that have other members).
	if _, err := tx.Exec(ctx, `DELETE FROM tenant_members WHERE user_id = $1`, userID); err != nil {
		slog.Error("account delete: tenant_members", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not purge account data")
		return
	}

	if err := tx.Commit(ctx); err != nil {
		slog.Error("account delete: commit", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not close account")
		return
	}

	// 2) Talos cleanup — best-effort. Non-fatal: if Talos is unreachable or
	// errors, the keys are unusable once the Kratos identity is gone (ext_authz
	// resolves the actor against Kratos), so we log and continue to the
	// authoritative identity deletion.
	talos := talosDB()
	if talos != nil {
		for _, t := range []string{"issued_api_keys", "imported_api_keys", "actor_balances", "api_key_usage"} {
			if _, err := talos.Exec(ctx, `DELETE FROM `+t+` WHERE actor_id = $1`, userID); err != nil {
				slog.Error("account delete: talos purge (non-fatal)", "table", t, "user", userID, "err", err)
			}
		}
	} else {
		slog.Warn("account delete: TALOS_DSN unset, skipping Talos cleanup", "user", userID)
	}

	// 3) Kratos identity (loopback admin). Done last; idempotent. The base URL
	// is loopback-only and relies on network isolation (no token), matching how
	// the bootstrap webhook reaches pREST.
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

// kratosAdminURL resolves the Kratos admin API base (loopback :4434).
func kratosAdminURL() string {
	if v := os.Getenv("KRATOS_ADMIN_URL"); v != "" {
		return v
	}
	return "http://localhost:4434"
}

// ActiveWorkspaceSetHandler: PATCH /v1/account/active-workspace — sets the
// caller's active workspace in their Kratos identity metadata_public. The
// workspace is then read by Oathkeeper on every subsequent request (via
// whoami → metadata_public.active_workspace_id) and injected as X-Tenant-Id.
//
// The caller must be a member of the target workspace. This route is under
// the "bare" Oathkeeper rule (no workspace authz) because it SETS the
// workspace — it cannot require one to exist yet.
func ActiveWorkspaceSetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := extractUserID(r)
	if userID == "" || userID == "anonymous" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var in struct {
		WorkspaceID string `json:"workspaceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	if in.WorkspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspaceId required")
		return
	}

	ctx := r.Context()

	if !userIsMember(ctx, kawaiDB(), in.WorkspaceID, userID) {
		writeJSONError(w, http.StatusForbidden, "not a member of this workspace")
		return
	}

	if err := setKratosMetadata(ctx, userID, map[string]any{
		"active_workspace_id": in.WorkspaceID,
	}); err != nil {
		slog.Error("active-workspace: set metadata", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "failed to update identity")
		return
	}

	slog.Info("active workspace set", "user", userID, "workspace", in.WorkspaceID)
	writeJSON(w, http.StatusOK, map[string]any{"workspaceId": in.WorkspaceID})
}

// ActiveWorkspaceGetHandler: GET /v1/account/active-workspace — returns the
// caller's active workspace from Kratos identity metadata_public. The SPA
// uses this on page load to sync client state with the server-side source of
// truth.
func ActiveWorkspaceGetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	userID := extractUserID(r)
	if userID == "" || userID == "anonymous" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	meta, err := getKratosMetadata(r.Context(), userID)
	if err != nil {
		slog.Error("active-workspace: get metadata", "user", userID, "err", err)
		writeJSONError(w, http.StatusBadGateway, "failed to read identity")
		return
	}

	wsID, _ := meta["active_workspace_id"].(string)
	writeJSON(w, http.StatusOK, map[string]any{
		"workspaceId": wsID,
	})
}

// getKratosMetadata fetches identity.metadata_public from the Kratos admin API.
func getKratosMetadata(ctx context.Context, userID string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		kratosAdminURL()+"/admin/identities/"+userID, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kratos get identity HTTP %d", resp.StatusCode)
	}
	var identity struct {
		MetadataPublic map[string]any `json:"metadata_public"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&identity); err != nil {
		return nil, err
	}
	if identity.MetadataPublic == nil {
		return map[string]any{}, nil
	}
	return identity.MetadataPublic, nil
}

// setKratosMetadata updates identity.metadata_public via GET + PUT on the
// Kratos admin API. Existing metadata keys are preserved; new keys are merged.
func setKratosMetadata(ctx context.Context, userID string, updates map[string]any) error {
	base := kratosAdminURL()

	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/admin/identities/"+userID, nil)
	if err != nil {
		return err
	}
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		return err
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		return fmt.Errorf("kratos get identity HTTP %d", getResp.StatusCode)
	}

	var identity map[string]any
	if err := json.NewDecoder(getResp.Body).Decode(&identity); err != nil {
		return err
	}

	existing, _ := identity["metadata_public"].(map[string]any)
	if existing == nil {
		existing = map[string]any{}
	}
	for k, v := range updates {
		existing[k] = v
	}

	// Kratos admin PUT validates strictly and rejects read-only fields
	// (credentials, *_at timestamps, schema_url, organization_id, …) that the
	// GET returns. Reconstruct the PUT body from only the mutable fields the
	// admin API accepts: schema_id, traits, state, metadata_public, metadata_admin.
	putBody := map[string]any{
		"metadata_public": existing,
	}
	for _, k := range []string{"schema_id", "traits", "state", "metadata_admin"} {
		if v, ok := identity[k]; ok {
			putBody[k] = v
		}
	}

	body, err := json.Marshal(putBody)
	if err != nil {
		return err
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		base+"/admin/identities/"+userID, bytes.NewReader(body))
	if err != nil {
		return err
	}
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		return err
	}
	defer putResp.Body.Close()
	io.Copy(io.Discard, putResp.Body)
	if putResp.StatusCode != http.StatusOK {
		return fmt.Errorf("kratos put identity HTTP %d", putResp.StatusCode)
	}
	return nil
}
