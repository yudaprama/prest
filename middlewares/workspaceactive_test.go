package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/stretchr/testify/require"
)

func TestTenantActiveMiddleware_NoConfig(t *testing.T) {
	config.PrestConf.TenantActiveHeader = ""

	middleware := TenantActiveMiddleware()

	var received string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(pctx.TenantIDActiveKey).(string); ok {
			received = v
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-Id", "ws-7")
	middleware.ServeHTTP(httptest.NewRecorder(), req, handler)

	require.Empty(t, received, "active tenant must not be set when header config is empty")
}

func TestTenantActiveMiddleware_HeaderPresent(t *testing.T) {
	config.PrestConf.TenantActiveHeader = "X-Tenant-Id"
	defer func() { config.PrestConf.TenantActiveHeader = "" }()

	middleware := TenantActiveMiddleware()

	var received string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(pctx.TenantIDActiveKey).(string); ok {
			received = v
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Tenant-Id", "ws-7")
	middleware.ServeHTTP(httptest.NewRecorder(), req, handler)

	require.Equal(t, "ws-7", received, "active tenant id should be threaded into context")
}

func TestTenantActiveMiddleware_EmptyHeaderPersonalMode(t *testing.T) {
	config.PrestConf.TenantActiveHeader = "X-Tenant-Id"
	defer func() { config.PrestConf.TenantActiveHeader = "" }()

	middleware := TenantActiveMiddleware()

	var received string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(pctx.TenantIDActiveKey).(string); ok {
			received = v
		}
		w.WriteHeader(http.StatusOK)
	})

	// No header at all → personal mode (key not set).
	req := httptest.NewRequest("GET", "/test", nil)
	middleware.ServeHTTP(httptest.NewRecorder(), req, handler)
	require.Empty(t, received, "absent header means personal mode; key must be unset")
}
