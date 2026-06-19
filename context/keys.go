package context

type Key int

const (
	_ Key = iota
	DBNameKey
	HTTPTimeoutKey
	UserInfoKey
	UserIDKey
	// WorkspaceIDKey holds the single workspace ID selected for a
	// request when `?workspaceId=…` is present AND authorized via Keto
	// (WorkspaceAuthzGate). Templates read it via the `workspaceId`
	// template var. Empty when not set; the request then runs in
	// personal scope (workspace_id IS NULL) or cross-workspace scope.
	WorkspaceIDKey
	// WorkspaceIDsKey holds the full list of workspace IDs the caller
	// is a member of, resolved once per request by
	// WorkspaceMembershipResolver (Keto ListObjects, cached). Used by
	// the postgres adapter to inject `WHERE workspace_id IN (...)` on
	// Tier 1 workspace tables, and by the `workspaceScopeIn` template
	// helper for cross-workspace Tier 2 reads.
	WorkspaceIDsKey
)
