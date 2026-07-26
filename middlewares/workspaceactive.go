package middlewares

import (
	"context"
	"net/http"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/urfave/negroni/v3"
)

// TenantActiveMiddleware copies the single active tenant id from the
// configured request header (default "X-Tenant-Id") into
// pctx.TenantIDActiveKey. The header is set by the frontend after Oathkeeper
// authorizes the tenant (pREST POST /authz/workspace), so its
// presence is the trusted signal that the caller is authorized for that
// tenant. An absent or empty header means personal mode (no tenant
// active). Used by the "compat" filter mode; makes no authz calls itself.
func TenantActiveMiddleware() negroni.Handler {
	return negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		if config.PrestConf.TenantActiveHeader == "" {
			next(rw, r)
			return
		}
		if ws := r.Header.Get(config.PrestConf.TenantActiveHeader); ws != "" {
			ctx := context.WithValue(r.Context(), pctx.TenantIDActiveKey, ws)
			next(rw, r.WithContext(ctx))
			return
		}
		next(rw, r)
	})
}
