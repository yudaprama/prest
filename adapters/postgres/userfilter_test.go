package postgres

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/stretchr/testify/require"
)

func TestResolveUserIDColumn_NoFilters(t *testing.T) {
	config.PrestConf.UserIDFilters = nil

	req := httptest.NewRequest("GET", "/yarsew/public/billing_balances", nil)
	column := ResolveUserIDColumn(req)
	require.Empty(t, column, "Should return empty when no filters configured")
}

func TestResolveUserIDColumn_MatchFound(t *testing.T) {
	t.Run("by exact database+schema+table", func(t *testing.T) {
		config.PrestConf.UserIDFilters = []config.UserFilterConfig{
			{Database: "yarsew", Schema: "public", Table: "billing_balances", Column: "actor_id"},
		}
		req := httptest.NewRequest("GET", "/yarsew/public/billing_balances", nil)
		column := ResolveUserIDColumn(req)
		require.Equal(t, "actor_id", column)
	})

	t.Run("by exact different database", func(t *testing.T) {
		config.PrestConf.UserIDFilters = []config.UserFilterConfig{
			{Database: "yarsew", Schema: "public", Table: "billing_balances", Column: "actor_id"},
			{Database: "ogmami", Schema: "public", Table: "sessions", Column: "identity_id"},
		}
		req := httptest.NewRequest("GET", "/ogmami/public/sessions", nil)
		column := ResolveUserIDColumn(req)
		require.Equal(t, "identity_id", column)
	})

	t.Run("by different table", func(t *testing.T) {
		config.PrestConf.UserIDFilters = []config.UserFilterConfig{
			{Database: "yarsew", Schema: "public", Table: "billing_balances", Column: "actor_id"},
		}
		req := httptest.NewRequest("GET", "/yarsew/public/conversation_states", nil)
		column := ResolveUserIDColumn(req)
		require.Empty(t, column, "Should return empty for non-configured table")
	})

	t.Run("by different schema", func(t *testing.T) {
		config.PrestConf.UserIDFilters = []config.UserFilterConfig{
			{Database: "yarsew", Schema: "public", Table: "billing_balances", Column: "actor_id"},
		}
		req := httptest.NewRequest("GET", "/yarsew/private/billing_balances", nil)
		column := ResolveUserIDColumn(req)
		require.Empty(t, column, "Schema must match exactly")
	})
}

func TestResolveUserIDColumn_PathFormats(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "yarsew", Schema: "public", Table: "billing_balances", Column: "actor_id"},
	}

	t.Run("path with query parameters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/billing_balances?status=pending&page=1", nil)
		column := ResolveUserIDColumn(req)
		require.Equal(t, "actor_id", column)
	})

	t.Run("path with fewer segments", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/databases", nil)
		column := ResolveUserIDColumn(req)
		require.Empty(t, column)
	})

	t.Run("root path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		column := ResolveUserIDColumn(req)
		require.Empty(t, column)
	})

	t.Run("health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/_health", nil)
		column := ResolveUserIDColumn(req)
		require.Empty(t, column)
	})
}

func TestWhereByRequest_WithUserFilter(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "yarsew", Schema: "public", Table: "billing_balances", Column: "actor_id"},
	}

	t.Run("injects user_id filter when header present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/billing_balances?status=pending", nil)
		ctx := context.WithValue(req.Context(), pctx.UserIDKey, "user-123")
		req = req.WithContext(ctx)

		adapter := &Postgres{}
		where, values, err := adapter.WhereByRequest(req, 1)

		require.NoError(t, err)
		require.Contains(t, where, `"actor_id" = $1`)
		require.Contains(t, where, `"status" = $2`)
		require.Contains(t, values, "user-123")
	})

	t.Run("skips when no user_id in context", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/billing_balances?status=pending", nil)

		adapter := &Postgres{}
		where, values, err := adapter.WhereByRequest(req, 1)

		require.NoError(t, err)
		require.NotContains(t, where, "actor_id")
		require.NotContains(t, values, "user-123")
	})

	t.Run("skips when table not configured", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/conversation_states?status=pending", nil)
		ctx := context.WithValue(req.Context(), pctx.UserIDKey, "user-123")
		req = req.WithContext(ctx)

		adapter := &Postgres{}
		where, values, err := adapter.WhereByRequest(req, 1)

		require.NoError(t, err)
		require.NotContains(t, where, "user_id")
		require.NotContains(t, where, "actor_id")
		require.NotContains(t, values, "user-123")
	})

	t.Run("sets correct placeholder numbering", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/billing_balances?status=pending", nil)
		ctx := context.WithValue(req.Context(), pctx.UserIDKey, "user-abc")
		req = req.WithContext(ctx)

		adapter := &Postgres{}
		where, _, err := adapter.WhereByRequest(req, 5)

		require.NoError(t, err)
		require.Contains(t, where, `= $5`, "User filter should use initial placeholder")
		require.Contains(t, where, `= $6`, "Status param should use next placeholder")
	})

	t.Run("works with multiple _where params", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/billing_balances?status=pending&type=premium", nil)
		ctx := context.WithValue(req.Context(), pctx.UserIDKey, "user-xyz")
		req = req.WithContext(ctx)

		adapter := &Postgres{}
		where, values, err := adapter.WhereByRequest(req, 1)

		require.NoError(t, err)
		require.Contains(t, where, `"actor_id" = $1`)
		require.Contains(t, where, `"status" = $2`)
		require.Contains(t, where, `"type" = $3`)
		require.Contains(t, values, "user-xyz")
	})
}

func TestWhereByRequest_UserFilterWithOrClause(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "yarsew", Schema: "public", Table: "orders", Column: "user_id"},
	}

	t.Run("user filter works with _or clause (|| separator)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/orders?_or=status=pending||status=active", nil)
		ctx := context.WithValue(req.Context(), pctx.UserIDKey, "user-456")
		req = req.WithContext(ctx)

		adapter := &Postgres{}
		where, values, err := adapter.WhereByRequest(req, 1)

		require.NoError(t, err)
		require.Contains(t, where, `"user_id" = $1`)
		require.Contains(t, where, "OR")
		require.Contains(t, values, "user-456")
	})

	t.Run("user filter works with _or clause (OR keyword)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/yarsew/public/orders?_or=status=pending%20OR%20status=active", nil)
		ctx := context.WithValue(req.Context(), pctx.UserIDKey, "user-456")
		req = req.WithContext(ctx)

		adapter := &Postgres{}
		where, values, err := adapter.WhereByRequest(req, 1)

		require.NoError(t, err)
		require.Contains(t, where, `"user_id" = $1`)
		require.Contains(t, where, "OR")
		require.Contains(t, values, "user-456")
	})
}

func TestWhereByRequest_UserFilterWithMultipleParams(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "ogmami", Schema: "public", Table: "sessions", Column: "identity_id"},
	}

	t.Run("user filter applied before other params", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ogmami/public/sessions?active=true", nil)
		ctx := context.WithValue(req.Context(), pctx.UserIDKey, "identity-789")
		req = req.WithContext(ctx)

		adapter := &Postgres{}
		where, values, err := adapter.WhereByRequest(req, 1)

		require.NoError(t, err)
		require.Contains(t, where, `"identity_id" = $1`)
		require.Contains(t, where, `"active" = $2`)
		require.Contains(t, values, "identity-789")
		require.Contains(t, values, "true")
	})
}
