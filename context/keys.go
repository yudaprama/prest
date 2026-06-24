package context

type Key int

const (
	_ Key = iota
	DBNameKey
	HTTPTimeoutKey
	UserInfoKey
	UserIDKey
	// WorkspaceIDKey holds the single workspace ID for a request, exposed
	// to SQL templates via the `workspaceId` var. NOTE: its only setter
	// (the WorkspaceAuthzGate middleware, which read `?workspaceId=`)
	// has been removed — authentication and workspace authorization now
	// live in Ory Oathkeeper. This key is therefore vestigial (always
	// empty) and kept only because controllers/sql.go still reads it; it
	// is harmlessly empty unless repopulated from the X-Workspace-Id
	// header in a future change. Active-workspace scoping now flows via
	// WorkspaceIDActiveKey; union membership via WorkspaceIDsKey.
	WorkspaceIDKey
	// WorkspaceIDsKey holds the full list of workspace IDs the caller
	// is a member of, resolved once per request by
	// WorkspaceMembershipResolver (Keto ListObjects, cached). Used by
	// the postgres adapter to inject `WHERE workspace_id IN (...)` on
	// Tier 1 workspace tables, and by the `workspaceScopeIn` template
	// helper for cross-workspace Tier 2 reads.
	WorkspaceIDsKey
	// WorkspaceIDActiveKey holds the single active workspace id for the
	// request, sourced from the X-Workspace-Id header. Oathkeeper sets
	// this header authoritatively (after its own Keto Check); empty =
	// personal mode. Used exclusively by the "compat" filter mode
	// (buildWorkspaceWhere semantics) on workspace-capable content
	// tables — distinct from WorkspaceIDsKey (union membership) and
	// WorkspaceIDKey (vestigial single-value template var).
	WorkspaceIDActiveKey
)
