package postgres

import (
	"net/http"
	"strings"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
)

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