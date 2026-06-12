package postgres

import (
	"net/http"
	"strings"

	"github.com/prest/prest/v2/config"
)

// resolveUserIDColumn returns the configured user_id column for the current request
// Format: lookup by database.schema.table from config
func resolveUserIDColumn(r *http.Request) string {
	if len(config.PrestConf.UserIDFilters) == 0 {
		return ""
	}

	// Extract database, schema, table from URL path: /{database}/{schema}/{table}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	database := parts[0]
	schema := parts[1]
	table := parts[2]

	// Find matching config
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
