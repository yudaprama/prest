package controllers

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/scrypster/muninndb/sdk/go/muninn"
)

var (
	muninnClient     *muninn.Client
	muninnClientOnce sync.Once
)

// muninnClientFromEnv returns a shared MuninnDB client, or nil when
// MUNINN_URL is unset (dev/CI). The client is created once and reused.
// Auth is via the edge-auth trust header (set per-request by callers through
// muninn.WithTrustedVaultHeader), not a bearer token — MuninnDB must run in
// edge-auth mode (MUNINN_TRUST_EDGE_HEADER) for writes to be authorized.
func muninnClientFromEnv() *muninn.Client {
	muninnClientOnce.Do(func() {
		url := os.Getenv("MUNINN_URL")
		if url == "" {
			return
		}
		muninnClient = muninn.NewClient(url, "")
	})
	return muninnClient
}

// ingestProfileToMemory writes registration-time profile facts to MuninnDB.
// Best-effort: failures are logged but never block the caller.
// tenantID is the workspace where the user first created a workspace;
// userID is the Kratos identity ID.
func ingestProfileToMemory(ctx context.Context, tenantID, userID, email string) {
	// Detach from the request context so the write survives after the HTTP
	// response is sent (request ctx is cancelled at that point).
	ctx = context.WithoutCancel(ctx)
	// Authorize via the edge-auth trust header — the same mechanism the egents
	// use for reads — so this write is bound to the named tenant rather than
	// relying solely on a global bearer token. Honored when MuninnDB runs in
	// edge-auth mode (MUNINN_TRUST_EDGE_HEADER); harmless otherwise.
	ctx = muninn.WithTrustedVaultHeader(ctx, muninn.TrustEdgeHeaderFromEnv(), tenantID)

	client := muninnClientFromEnv()
	if client == nil {
		return
	}

	facts := map[string]string{
		"user.email": email,
	}
	if email != "" {
		// Derive a display name from the email prefix (before @).
		if at := strings.IndexByte(email, '@'); at > 0 {
			facts["user.name"] = email[:at]
		}
	}

	for key, value := range facts {
		if _, err := client.Write(ctx, tenantID, key, value, muninn.ProfileTags(key, userID)); err != nil {
			slog.Warn("ingest profile: write failed", "key", key, "err", err)
		}
	}
}
