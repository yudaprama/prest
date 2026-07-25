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

// --- active-tenant ("compat") mode: ResolveTenantCompat ---

func TestResolveTenantCompat_NoFilters(t *testing.T) {
	config.PrestConf.TenantCompatFilters = nil
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	if postgres.ResolveTenantCompat(req) != nil {
		t.Fatal("expected nil when no compat filters configured")
	}
}

func TestResolveTenantCompat_MatchFound(t *testing.T) {
	config.PrestConf.TenantCompatFilters = []config.TenantCompatConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", TenantColumn: "tenant_id"},
	}
	defer func() { config.PrestConf.TenantCompatFilters = nil }()
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	got := postgres.ResolveTenantCompat(req)
	if got == nil || got.TenantColumn != "tenant_id" || got.UserColumn != "user_id" {
		t.Fatalf("expected matched compat config, got %+v", got)
	}
}

func TestResolveTenantCompat_NoMatch(t *testing.T) {
	config.PrestConf.TenantCompatFilters = []config.TenantCompatConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", TenantColumn: "tenant_id"},
	}
	defer func() { config.PrestConf.TenantCompatFilters = nil }()
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	if postgres.ResolveTenantCompat(req) != nil {
		t.Fatal("expected nil for a table not in compat filters")
	}
}

func TestTenantIDActiveFromContext_NotSet(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	if postgres.TenantIDActiveFromContext(req) != "" {
		t.Fatal("expected empty when key absent")
	}
}

func TestTenantIDActiveFromContext_Populated(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.TenantIDActiveKey, "ws-9"))
	if got := postgres.TenantIDActiveFromContext(req); got != "ws-9" {
		t.Fatalf("expected ws-9, got %q", got)
	}
}

// --- active-tenant ("compat") mode: WhereByRequest injection ---

func compatDocsConfig() []config.TenantCompatConfig {
	return []config.TenantCompatConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", TenantColumn: "tenant_id"},
	}
}

func TestWhereByRequest_Compat_ActiveTenant(t *testing.T) {
	config.PrestConf.TenantCompatFilters = compatDocsConfig()
	defer func() { config.PrestConf.TenantCompatFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "user-1"))
	req = req.WithContext(context.WithValue(req.Context(), pctx.TenantIDActiveKey, "ws-9"))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, `"tenant_id" = $1`) {
		t.Fatalf("expected active-tenant clause, got %q", where)
	}
	if strings.Contains(where, `"user_id" =`) {
		t.Fatalf("active-tenant mode must NOT also emit a user_id clause, got %q", where)
	}
	if len(values) != 1 || values[0] != "ws-9" {
		t.Fatalf("expected values=[ws-9], got %v", values)
	}
}

func TestWhereByRequest_Compat_PersonalMode(t *testing.T) {
	config.PrestConf.TenantCompatFilters = compatDocsConfig()
	defer func() { config.PrestConf.TenantCompatFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "user-1"))
	// No  TenantIDActiveKey → personal mode.

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, `"user_id" = $1`) || !strings.Contains(where, `"tenant_id" IS NULL`) {
		t.Fatalf("expected personal-mode clause, got %q", where)
	}
	if len(values) != 1 || values[0] != "user-1" {
		t.Fatalf("expected values=[user-1], got %v", values)
	}
}

func TestWhereByRequest_Compat_NoIdentity_FailOpen(t *testing.T) {
	config.PrestConf.TenantCompatFilters = compatDocsConfig()
	defer func() { config.PrestConf.TenantCompatFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	// Neither UserIDKey nor  TenantIDActiveKey set.

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(where, "tenant_id") || strings.Contains(where, "user_id") {
		t.Fatalf("fail-open: expected no compat clause with no identity, got %q", where)
	}
	if len(values) != 0 {
		t.Fatalf("expected no values, got %v", values)
	}
}

// Precedence invariant: a table must get exactly one user-column predicate.
// Even if (mis)configured in both user_id_filters and tenant_compat_filters,
// compat takes precedence — the plain user_id filter is suppressed.
func TestWhereByRequest_Compat_TakesPrecedenceOverUserIdFilter(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "documents", Column: "user_id"},
	}
	config.PrestConf.TenantCompatFilters = compatDocsConfig()
	defer func() {
		config.PrestConf.UserIDFilters = nil
		config.PrestConf.TenantCompatFilters = nil
	}()

	req := httptest.NewRequest("GET", "/lobehub/public/documents", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "user-1"))
	req = req.WithContext(context.WithValue(req.Context(), pctx.TenantIDActiveKey, "ws-9"))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(where, `"user_id" =`) != 0 {
		t.Fatalf("user_id filter must be suppressed under compat, got %q", where)
	}
	if !strings.Contains(where, `"tenant_id" = $1`) {
		t.Fatalf("expected compat active-tenant clause, got %q", where)
	}
	if len(values) != 1 || values[0] != "ws-9" {
		t.Fatalf("expected values=[ws-9], got %v", values)
	}
}

// --- ValidateTenantCompat (config overlap check) ---

func TestValidateTenantCompat_Disjoint(t *testing.T) {
	cfg := &config.Prest{
		UserIDFilters: []config.UserFilterConfig{
			{Database: "lobehub", Schema: "public", Table: "sessions", Column: "user_id"},
		},
		TenantCompatFilters: []config.TenantCompatConfig{
			{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", TenantColumn: "tenant_id"},
		},
	}
	if err := config.ValidateTenantCompat(cfg); err != nil {
		t.Fatalf("expected nil for disjoint sets, got %v", err)
	}
}

func TestValidateTenantCompat_OverlapRejected(t *testing.T) {
	cfg := &config.Prest{
		UserIDFilters: []config.UserFilterConfig{
			{Database: "lobehub", Schema: "public", Table: "documents", Column: "user_id"},
		},
		TenantCompatFilters: []config.TenantCompatConfig{
			{Database: "lobehub", Schema: "public", Table: "documents", UserColumn: "user_id", TenantColumn: "tenant_id"},
		},
	}
	if err := config.ValidateTenantCompat(cfg); err == nil {
		t.Fatal("expected error when a table is in both user_id_filters and tenant_compat_filters")
	}
}
