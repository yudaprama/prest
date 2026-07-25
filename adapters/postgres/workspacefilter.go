package postgres

import (
	"net/http"
	"strings"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
)

// ResolveTenantCompat returns the active-tenant ("compat") config for
// the current request's {database}/{schema}/{table}, or nil if no entry
// matches. When non-nil, WhereByRequest emits buildTenantWhere semantics
// for this table instead of the plain user_id filter.
func ResolveTenantCompat(r *http.Request) *config.TenantCompatConfig {
	if len(config.PrestConf.TenantCompatFilters) == 0 {
		return nil
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return nil
	}
	database := parts[0]
	schema := parts[1]
	table := parts[2]

	for i := range config.PrestConf.TenantCompatFilters {
		f := &config.PrestConf.TenantCompatFilters[i]
		if f.Database == database &&
			(f.Schema == "" || f.Schema == schema) &&
			f.Table == table &&
			f.UserColumn != "" && f.TenantColumn != "" {
			return f
		}
	}
	return nil
}

//	TenantIDActiveFromContext returns the single active tenant id for the
//
// request (from pctx.TenantIDActiveKey, set by TenantActiveMiddleware
// from the X-Tenant-Id header). Empty string = personal mode.
func TenantIDActiveFromContext(r *http.Request) string {
	ws, ok := r.Context().Value(pctx.TenantIDActiveKey).(string)
	if !ok {
		return ""
	}
	return ws
}
