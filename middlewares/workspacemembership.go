package middlewares

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/urfave/negroni/v3"
	keto "github.com/yudaprama/authzworkspace"
)

// Default cache tuning. 30s TTL keeps the Keto call cost per request
// low (max ~1 ListObjects per user per 30s) while keeping the
// staleness window acceptable for read paths. Writes still go through
// uncached Keto Check in the BFF.
const (
	membershipCacheTTL     = 30 * time.Second
	membershipCacheMaxSize = 10000
)

// membershipCache is shared across resolver instances. It is keyed by
// userID and holds the deduplicated list of workspace IDs from Keto.
var membershipCache = NewTTLCache(membershipCacheMaxSize, membershipCacheTTL)

// WorkspaceMembershipResolver is a Phase 2 middleware that resolves the
// caller's full workspace membership once per request (with a 30s LRU
// cache) and stores it in pctx.WorkspaceIDsKey.
//
// The resolved list is used by:
//   - adapters/postgres to inject `WHERE workspace_id IN (...)` on the
//     four workspace tables configured in `[[auth.workspace_id_filters]]`.
//   - SQL templates via the `workspaceIds` template variable and the
//     `workspaceScopeIn` helper for cross-workspace reads.
//
// When Keto is unavailable or the request has no authenticated
// identity, the middleware sets an empty list on the context. This is
// fail-closed for the four workspace tables (they return nothing) but
// fail-open for personal-scope tables (their existing user_id filter
// still applies).
//
// The resolver is gated by the `auth.workspace_filters_enabled` flag.
// When disabled, it is a pass-through to keep Phase 1 behavior intact.
func WorkspaceMembershipResolver() negroni.Handler {
	client := keto.New(config.PrestConf.KetoReadURL, config.PrestConf.KetoWriteURL)

	return negroni.HandlerFunc(func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		if !config.PrestConf.WorkspaceFiltersEnabled {
			next(rw, r)
			return
		}

		userID := userIDFromContext(r)
		if userID == "" {
			// No authenticated identity: set empty list so workspace
			// tables remain empty while personal-scope tables still
			// work (their user_id filter is silently skipped too).
			ctx := context.WithValue(r.Context(), pctx.WorkspaceIDsKey, []string{})
			next(rw, r.WithContext(ctx))
			return
		}

		workspaceIDs, err := resolveMembership(r.Context(), client, userID)
		if err != nil {
			slog.Warn("[keto] workspace membership resolution failed, blocking workspace tables", "userId", userID, "err", err)
			workspaceIDs = []string{}
		}

		ctx := context.WithValue(r.Context(), pctx.WorkspaceIDsKey, workspaceIDs)
		next(rw, r.WithContext(ctx))
	})
}

// resolveMembership looks up the user's workspace list with the cache.
func resolveMembership(ctx context.Context, client *keto.Client, userID string) ([]string, error) {
	if cached, ok := membershipCache.Get(userID); ok {
		return cached, nil
	}

	workspaceIDs, err := client.ListWorkspacesForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	membershipCache.Set(userID, workspaceIDs)
	return workspaceIDs, nil
}

// ResetMembershipCacheForTest clears the membership cache. Exposed only
// so tests in other packages can reset state between runs; not part of
// the public API and must not be called from production code.
func ResetMembershipCacheForTest() {
	membershipCache = NewTTLCache(membershipCacheMaxSize, membershipCacheTTL)
}
