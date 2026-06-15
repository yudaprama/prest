package postgres

import (
	"net/http"
	"strings"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
)

// ResolveUserIDColumn returns the configured user_id column for the
// current request. The lookup is by `{database}/{schema}/{table}`
// from the URL path; if no entry matches, the function returns an
// empty string and the filter is silently skipped.
//
// The intent is to keep the data-access layer unaware of which
// upstream auth scheme (basic, JWT, Kratos, header) populated the
// `pctx.UserIDKey` value — any middleware that sets the context key
// will cause the filter to be applied.
//
// Exported so that test packages and external middleware can drive
// it without going through the package internals.
func ResolveUserIDColumn(r *http.Request) string {
	if len(config.PrestConf.UserIDFilters) == 0 {
		return ""
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	database := parts[0]
	schema := parts[1]
	table := parts[2]

	for _, filter := range config.PrestConf.UserIDFilters {
		if filter.Database == database &&
			(filter.Schema == "" || filter.Schema == schema) &&
			filter.Table == table &&
			filter.Column != "" {
			return filter.Column
		}
	}

	return ""
}

// UserIDFromContext returns the authenticated identity ID stored on
// the request context, or an empty string if the upstream middleware
// did not set one. The filter is skipped when the value is empty so
// that public endpoints continue to work without an auth layer.
func UserIDFromContext(r *http.Request) string {
	if id, ok := r.Context().Value(pctx.UserIDKey).(string); ok {
		return id
	}
	return ""
}
