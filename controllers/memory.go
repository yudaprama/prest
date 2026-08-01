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
func muninnClientFromEnv() *muninn.Client {
	muninnClientOnce.Do(func() {
		url := os.Getenv("MUNINN_URL")
		if url == "" {
			return
		}
		muninnClient = muninn.NewClient(url, os.Getenv("MUNINN_TOKEN"))
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
		tags := []string{"user:" + userID, "profile"}
		if _, err := client.Write(ctx, tenantID, key, value, tags); err != nil {
			slog.Warn("ingest profile: write failed", "key", key, "err", err)
		}
	}
}
