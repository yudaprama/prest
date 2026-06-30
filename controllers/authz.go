package controllers

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/prest/prest/v2/config"

	authz "github.com/yudaprama/authzworkspace"
)

// Workspace role tiers (mirror of the workspace_members.role values).
const (
	roleOwner  = "owner"
	roleMember = "member"
	roleViewer = "viewer"
)

// AuthzWorkspaceHandler builds the /authz/workspace status-code adapter that the
// Ory Oathkeeper edge calls via remote_json (prest-workspace-* and
// hatchet-workspace-* rules). Keto is the source of truth; it falls back to the
// workspace_members table (via pREST's own DB) when Keto is not configured —
// preserving the behaviour previously hosted by egent-lobehub. Built once at
// router setup. Hosting it here means the edge keystone survives egent-lobehub's
// removal without a new standalone service.
func AuthzWorkspaceHandler() http.Handler {
	client := authz.New(config.PrestConf.KetoReadURL, config.PrestConf.KetoWriteURL)
	return authz.HTTPHandler(client, workspaceMembershipFallback)
}

// workspaceMembershipFallback authorizes a workspace permission against the
// workspace_members table when Keto is disabled. Returns (false, nil) for
// non-members (deny, not an error).
func workspaceMembershipFallback(ctx context.Context, workspace, user, perm string) (bool, error) {
	if workspace == "" || user == "" {
		return false, nil
	}
	db, err := kawaiDB()
	if err != nil {
		return false, err
	}
	var role string
	err = db.QueryRowContext(ctx,
		`SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`,
		workspace, user).Scan(&role)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	switch perm {
	case "manage":
		return role == roleOwner, nil
	case "write":
		return role == roleOwner || role == roleMember, nil
	default: // view
		return role == roleOwner || role == roleMember || role == roleViewer, nil
	}
}
