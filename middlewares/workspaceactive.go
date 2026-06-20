package middlewares

import (
	"context"
	"net/http"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/urfave/negroni/v3"
)

// WorkspaceActiveMiddleware copies the single active workspace id from the
// configured request header (default "X-Workspace-Id") into
// pctx.WorkspaceIDActiveKey. The header is set by the TS BFF after it has
// run its own Keto Check, so its presence is the trusted signal that the
// caller is authorized for that workspace. An absent or empty header means
// personal mode (no workspace active). Used by the "compat" filter mode;
// makes no Keto calls itself.
func WorkspaceActiveMiddleware() negroni.Handler {
	return negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		if config.PrestConf.WorkspaceActiveHeader == "" {
			next(rw, r)
			return
		}
		if ws := r.Header.Get(config.PrestConf.WorkspaceActiveHeader); ws != "" {
			ctx := context.WithValue(r.Context(), pctx.WorkspaceIDActiveKey, ws)
			next(rw, r.WithContext(ctx))
			return
		}
		next(rw, r)
	})
}
