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

func TestResolveUserIDColumn_Match(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "sessions", Column: "user_id"},
	}
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	if got := postgres.ResolveUserIDColumn(req); got != "user_id" {
		t.Fatalf("ResolveUserIDColumn = %q, want %q", got, "user_id")
	}
}

func TestResolveUserIDColumn_WildcardSchema(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Table: "sessions", Column: "user_id"},
	}
	req := httptest.NewRequest("GET", "/lobehub/whatever/sessions", nil)
	if got := postgres.ResolveUserIDColumn(req); got != "user_id" {
		t.Fatalf("ResolveUserIDColumn = %q, want %q (schema wildcard)", got, "user_id")
	}
}

func TestResolveUserIDColumn_NoMatch(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "sessions", Column: "user_id"},
	}
	req := httptest.NewRequest("GET", "/lobehub/public/messages", nil)
	if got := postgres.ResolveUserIDColumn(req); got != "" {
		t.Fatalf("ResolveUserIDColumn = %q, want empty", got)
	}
}

func TestResolveUserIDColumn_EmptyConfig(t *testing.T) {
	config.PrestConf.UserIDFilters = nil
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	if got := postgres.ResolveUserIDColumn(req); got != "" {
		t.Fatalf("ResolveUserIDColumn = %q, want empty (no rules)", got)
	}
}

func TestResolveUserIDColumn_EmptyColumn(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "sessions", Column: ""},
	}
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	if got := postgres.ResolveUserIDColumn(req); got != "" {
		t.Fatalf("ResolveUserIDColumn = %q, want empty (no column)", got)
	}
}

func TestResolveUserIDColumn_ShortPath(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "sessions", Column: "user_id"},
	}
	req := httptest.NewRequest("GET", "/lobehub/public", nil)
	if got := postgres.ResolveUserIDColumn(req); got != "" {
		t.Fatalf("ResolveUserIDColumn = %q, want empty (path too short)", got)
	}
}

func TestUserIDFromContext_Set(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "identity-1"))
	if got := postgres.UserIDFromContext(req); got != "identity-1" {
		t.Fatalf("UserIDFromContext = %q, want %q", got, "identity-1")
	}
}

func TestUserIDFromContext_Empty(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	if got := postgres.UserIDFromContext(req); got != "" {
		t.Fatalf("UserIDFromContext = %q, want empty", got)
	}
}
