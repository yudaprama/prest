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
	// WorkspaceIDActiveKey.
	WorkspaceIDKey
	// WorkspaceIDActiveKey holds the single active workspace id for the
	// request, sourced from the X-Workspace-Id header. Oathkeeper sets
	// this header authoritatively; empty =
	// personal mode. Used exclusively by the "compat" filter mode
	// (buildWorkspaceWhere semantics) on workspace-capable content
	// tables — distinct from WorkspaceIDsKey (union membership) and
	// WorkspaceIDKey (vestigial single-value template var).
	WorkspaceIDActiveKey
)
