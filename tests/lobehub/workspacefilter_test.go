// Package lobehubtest holds unit tests for the LobeHub migration
// additions in pREST that do not require a live database connection.
//
// They live outside `adapters/postgres` so they do not trigger the
// package's `init()` (which calls `adapters/postgres.Load()` and
// `os.Exit(1)` on connection failure).
package lobehubtest

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prest/prest/v2/adapters/postgres"
	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
)

func init() {
	if config.PrestConf == nil {
		config.PrestConf = &config.Prest{}
	}
}

// --- active-workspace ("compat") mode: ResolveWorkspaceCompat ---

func TestResolveWorkspaceCompat_NoFilters(t *testing.T) {
	config.PrestConf.WorkspaceCompatFilters = nil
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	if postgres.ResolveWorkspaceCompat(req) != nil {
		t.Fatal("expected nil when no compat filters configured")
	}
}

func TestResolveWorkspaceCompat_MatchFound(t *testing.T) {
	config.PrestConf.WorkspaceCompatFilters = []config.WorkspaceCompatConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", WorkspaceColumn: "workspace_id"},
	}
	defer func() { config.PrestConf.WorkspaceCompatFilters = nil }()
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	got := postgres.ResolveWorkspaceCompat(req)
	if got == nil || got.WorkspaceColumn != "workspace_id" || got.UserColumn != "user_id" {
		t.Fatalf("expected matched compat config, got %+v", got)
	}
}

func TestResolveWorkspaceCompat_NoMatch(t *testing.T) {
	config.PrestConf.WorkspaceCompatFilters = []config.WorkspaceCompatConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", WorkspaceColumn: "workspace_id"},
	}
	defer func() { config.PrestConf.WorkspaceCompatFilters = nil }()
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	if postgres.ResolveWorkspaceCompat(req) != nil {
		t.Fatal("expected nil for a table not in compat filters")
	}
}

func TestWorkspaceIDActiveFromContext_NotSet(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	if postgres.WorkspaceIDActiveFromContext(req) != "" {
		t.Fatal("expected empty when key absent")
	}
}

func TestWorkspaceIDActiveFromContext_Populated(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDActiveKey, "ws-9"))
	if got := postgres.WorkspaceIDActiveFromContext(req); got != "ws-9" {
		t.Fatalf("expected ws-9, got %q", got)
	}
}

// --- active-workspace ("compat") mode: WhereByRequest injection ---

func compatDocsConfig() []config.WorkspaceCompatConfig {
	return []config.WorkspaceCompatConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", WorkspaceColumn: "workspace_id"},
	}
}

func TestWhereByRequest_Compat_ActiveWorkspace(t *testing.T) {
	config.PrestConf.WorkspaceCompatFilters = compatDocsConfig()
	defer func() { config.PrestConf.WorkspaceCompatFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "user-1"))
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDActiveKey, "ws-9"))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, `"workspace_id" = $1`) {
		t.Fatalf("expected active-workspace clause, got %q", where)
	}
	if strings.Contains(where, `"user_id" =`) {
		t.Fatalf("active-workspace mode must NOT also emit a user_id clause, got %q", where)
	}
	if len(values) != 1 || values[0] != "ws-9" {
		t.Fatalf("expected values=[ws-9], got %v", values)
	}
}

func TestWhereByRequest_Compat_PersonalMode(t *testing.T) {
	config.PrestConf.WorkspaceCompatFilters = compatDocsConfig()
	defer func() { config.PrestConf.WorkspaceCompatFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "user-1"))
	// No WorkspaceIDActiveKey → personal mode.

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, `"user_id" = $1`) || !strings.Contains(where, `"workspace_id" IS NULL`) {
		t.Fatalf("expected personal-mode clause, got %q", where)
	}
	if len(values) != 1 || values[0] != "user-1" {
		t.Fatalf("expected values=[user-1], got %v", values)
	}
}

func TestWhereByRequest_Compat_NoIdentity_FailOpen(t *testing.T) {
	config.PrestConf.WorkspaceCompatFilters = compatDocsConfig()
	defer func() { config.PrestConf.WorkspaceCompatFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	// Neither UserIDKey nor WorkspaceIDActiveKey set.

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(where, "workspace_id") || strings.Contains(where, "user_id") {
		t.Fatalf("fail-open: expected no compat clause with no identity, got %q", where)
	}
	if len(values) != 0 {
		t.Fatalf("expected no values, got %v", values)
	}
}

// Precedence invariant: a table must get exactly one user-column predicate.
// Even if (mis)configured in both user_id_filters and workspace_compat_filters,
// compat takes precedence — the plain user_id filter is suppressed.
func TestWhereByRequest_Compat_TakesPrecedenceOverUserIdFilter(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", Column: "user_id"},
	}
	config.PrestConf.WorkspaceCompatFilters = compatDocsConfig()
	defer func() {
		config.PrestConf.UserIDFilters = nil
		config.PrestConf.WorkspaceCompatFilters = nil
	}()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "user-1"))
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDActiveKey, "ws-9"))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(where, `"user_id" =`) != 0 {
		t.Fatalf("user_id filter must be suppressed under compat, got %q", where)
	}
	if !strings.Contains(where, `"workspace_id" = $1`) {
		t.Fatalf("expected compat active-workspace clause, got %q", where)
	}
	if len(values) != 1 || values[0] != "ws-9" {
		t.Fatalf("expected values=[ws-9], got %v", values)
	}
}

// --- ValidateWorkspaceCompat (config overlap check) ---

func TestValidateWorkspaceCompat_Disjoint(t *testing.T) {
	cfg := &config.Prest{
		UserIDFilters: []config.UserFilterConfig{
			{Database: "lobehub", Schema: "public", Table: "sessions", Column: "user_id"},
		},
		WorkspaceCompatFilters: []config.WorkspaceCompatConfig{
			{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", WorkspaceColumn: "workspace_id"},
		},
	}
	if err := config.ValidateWorkspaceCompat(cfg); err != nil {
		t.Fatalf("expected nil for disjoint sets, got %v", err)
	}
}

func TestValidateWorkspaceCompat_OverlapRejected(t *testing.T) {
	cfg := &config.Prest{
		UserIDFilters: []config.UserFilterConfig{
			{Database: "lobehub", Schema: "public", Table: "documents", Column: "user_id"},
		},
		WorkspaceCompatFilters: []config.WorkspaceCompatConfig{
			{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", WorkspaceColumn: "workspace_id"},
		},
	}
	if err := config.ValidateWorkspaceCompat(cfg); err == nil {
		t.Fatal("expected error when a table is in both user_id_filters and workspace_compat_filters")
	}
}