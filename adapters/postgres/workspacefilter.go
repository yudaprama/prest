package postgres

import (
	"net/http"
	"strings"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
)

// ResolveWorkspaceIDColumn returns the configured workspace_id column
// for the current request. The lookup is by `{database}/{schema}/{table}`
// from the URL path; if no entry matches, the function returns an empty
// string and the filter is silently skipped.
//
// Mirrors ResolveUserIDColumn for the workspace scope introduced in
// Phase 2 (see WORKSPACE_SCOPE_IMPLEMENTATION_PLAN.md).
func ResolveWorkspaceIDColumn(r *http.Request) string {
	if len(config.PrestConf.WorkspaceIDFilters) == 0 {
		return ""
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	database := parts[0]
	schema := parts[1]
	table := parts[2]

	for _, filter := range config.PrestConf.WorkspaceIDFilters {
		if filter.Database == database &&
			(filter.Schema == "" || filter.Schema == schema) &&
			filter.Table == table &&
			filter.Column != "" {
			return filter.Column
		}
	}

	return ""
}

// WorkspaceIDsFromContext returns the resolved workspace list for the
// caller, populated by WorkspaceMembershipResolver. Returns nil when
// not set, and an empty (non-nil) slice when the caller is in zero
// workspaces. The postgres adapter treats nil and empty differently:
//   - nil: filter resolver has not run (gate disabled, anonymous
//     request, etc.) — no IN/FALSE clause is emitted, matching
//     pre-Phase-2 behavior.
//   - non-nil empty: resolver ran and found zero workspaces — emit
//     `FALSE` (fail-closed for workspace tables).
//   - non-empty: emit `col IN ($1, $2, …)` with the list bound.
func WorkspaceIDsFromContext(r *http.Request) []string {
	ids, ok := r.Context().Value(pctx.WorkspaceIDsKey).([]string)
	if !ok {
		return nil
	}
	return ids
}

// ResolveWorkspaceCompat returns the active-workspace ("compat") config for
// the current request's {database}/{schema}/{table}, or nil if no entry
// matches. When non-nil, WhereByRequest emits buildWorkspaceWhere semantics
// for this table instead of the plain user_id filter.
func ResolveWorkspaceCompat(r *http.Request) *config.WorkspaceCompatConfig {
	if len(config.PrestConf.WorkspaceCompatFilters) == 0 {
		return nil
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return nil
	}
	database := parts[0]
	schema := parts[1]
	table := parts[2]

	for i := range config.PrestConf.WorkspaceCompatFilters {
		f := &config.PrestConf.WorkspaceCompatFilters[i]
		if f.Database == database &&
			(f.Schema == "" || f.Schema == schema) &&
			f.Table == table &&
			f.UserColumn != "" && f.WorkspaceColumn != "" {
			return f
		}
	}
	return nil
}

// WorkspaceIDActiveFromContext returns the single active workspace id for the
// request (from pctx.WorkspaceIDActiveKey, set by WorkspaceActiveMiddleware
// from the X-Workspace-Id header). Empty string = personal mode.
func WorkspaceIDActiveFromContext(r *http.Request) string {
	ws, ok := r.Context().Value(pctx.WorkspaceIDActiveKey).(string)
	if !ok {
		return ""
	}
	return ws
}