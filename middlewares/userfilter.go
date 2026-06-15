package middlewares

import (
	"context"
	"net/http"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/urfave/negroni/v3"
)

// UserFilterMiddleware extracts the user ID from the configured header
// and stores it in the request context for use in query filtering
func UserFilterMiddleware() negroni.Handler {
	return negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		// Check if user_id_header is configured
		if config.PrestConf.UserIDHeader == "" {
			next(rw, r)
			return
		}

		// Extract user ID from header
		userID := r.Header.Get(config.PrestConf.UserIDHeader)
		if userID == "" {
			next(rw, r)
			return
		}

		// Store user ID in context
		ctx := context.WithValue(r.Context(), pctx.UserIDKey, userID)
		next(rw, r.WithContext(ctx))
	})
}
