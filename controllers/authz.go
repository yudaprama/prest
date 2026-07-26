package controllers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

// AuthzWorkspaceRequest is the payload Oathkeeper's remote_json authorizer
// sends to /authz/workspace. The fields are driven by oathkeeper-access-rules.yml
// prest-tenant-read/write payload templates.
type AuthzWorkspaceRequest struct {
	User        string `json:"user"`        // email (from Kratos session)
	WorkspaceID string `json:"workspace_id"` // tenant UUID from X-Tenant-Id header
	Tenant      string `json:"tenant"`      // legacy field name (still accepted)
	Permission  string `json:"permission"`  // view|write|manage
}

// AuthzWorkspaceHandler is the Oathkeeper remote_json target. Returns
// {"allowed": true} (HTTP 200) when the user holds the requested permission
// on the workspace, or {"allowed": false} (HTTP 403) otherwise.
//
// Source of truth: tenant_members table in kawai DB (denormalized email column
// for an O(1) indexed lookup).
//
// Permission hierarchy: owner > member > viewer.
//   view    → any role
//   write   → owner or member
//   manage  → owner only
func AuthzWorkspaceHandler(w http.ResponseWriter, r *http.Request) {
	var req AuthzWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]bool{"allowed": false})
		return
	}
	wsID := req.WorkspaceID
	if wsID == "" {
		wsID = req.Tenant // backward compatibility
	}
	email := strings.TrimSpace(strings.ToLower(req.User))
	perm := strings.ToLower(req.Permission)
	if perm == "" {
		perm = "view"
	}
	if wsID == "" || email == "" {
		writeJSON(w, http.StatusOK, map[string]bool{"allowed": false})
		return
	}

	db, err := kawaiDB()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]bool{"allowed": false})
		return
	}
	defer db.Close()

	var role string
	err = db.QueryRowContext(r.Context(),
		`SELECT role FROM tenant_members WHERE tenant_id = $1 AND lower(email) = $2`,
		wsID, email).Scan(&role)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]bool{"allowed": false})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]bool{"allowed": false})
		return
	}

	allowed := roleMatchesPermission(role, perm)
	writeJSON(w, http.StatusOK, map[string]bool{"allowed": allowed})
}

// roleMatchesPermission encodes the owner > member > viewer hierarchy.
func roleMatchesPermission(role, perm string) bool {
	switch perm {
	case "manage":
		return role == "owner"
	case "write":
		return role == "owner" || role == "member"
	default: // "view" and anything else
		return role == "owner" || role == "member" || role == "viewer"
	}
}
