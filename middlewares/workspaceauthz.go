package middlewares

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/prest/prest/v2/internal/keto"
	"github.com/urfave/negroni/v3"
)

// WorkspaceAuthzGate is a Phase 1 authorization middleware for
// single-workscope SQL template reads.
//
// When the query string contains `workspaceId`, it asks Ory Keto
// whether the authenticated user has at least `view` on that
// workspace. On deny it returns 403. On Keto error it logs a warning
// and falls open (matching the TS BFF `rbacPermission.ts` policy).
//
// When `workspaceId` is absent, or when no Kratos identity is in the
// request context, the gate is skipped and the request falls back to
// personal scope (the SQL template handles this via `isSet "workspaceId"`).
//
// The gate is intended to be registered after UserFilterMiddleware
// so that pctx.UserIDKey is already populated from the upstream Kratos
// proxy. The resolved `workspaceId` is also stored in
// pctx.WorkspaceIDKey so that `controllers/sql.go` can expose it as the
// `workspaceId` template variable.
func WorkspaceAuthzGate() negroni.Handler {
	client := keto.New(config.PrestConf.KetoReadURL, config.PrestConf.KetoWriteURL)

	return negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		// Phase 1 is behind the [keto] enabled flag. Without it, the
		// middleware becomes a pass-through to avoid changing behavior.
		if !config.PrestConf.KetoEnabled {
			next(rw, r)
			return
		}

		userID := userIDFromContext(r)
		workspaceID := r.URL.Query().Get("workspaceId")

		// No workspace requested, or no authenticated identity:
		// fall back to personal scope (template's `isSet "workspaceId"` else branch).
		if workspaceID == "" || userID == "" {
			next(rw, r)
			return
		}

		allowed, err := client.CheckWorkspace(r.Context(), workspaceID, userID, keto.PermissionView)
		if err != nil {
			// Fail-open: mirror TS BFF policy to avoid blocking all
			// requests during a Keto outage. Revisit once Keto has
			// proven uptime.
			slog.Warn("[keto] workspace authorization check failed, allowing request", "workspaceId", workspaceID, "userId", userID, "err", err)
			ctx := context.WithValue(r.Context(), pctx.WorkspaceIDKey, workspaceID)
			next(rw, r.WithContext(ctx))
			return
		}

		if !allowed {
			slog.Warn("[keto] workspace access denied", "workspaceId", workspaceID, "userId", userID)
			http.Error(rw, `{"error": "workspace access denied"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), pctx.WorkspaceIDKey, workspaceID)
		next(rw, r.WithContext(ctx))
	})
}

func userIDFromContext(r *http.Request) string {
	if id, ok := r.Context().Value(pctx.UserIDKey).(string); ok {
		return id
	}
	return ""
}
