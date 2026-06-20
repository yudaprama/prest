package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceActiveMiddleware_NoConfig(t *testing.T) {
	config.PrestConf.WorkspaceActiveHeader = ""

	middleware := WorkspaceActiveMiddleware()

	var received string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(pctx.WorkspaceIDActiveKey).(string); ok {
			received = v
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Workspace-Id", "ws-7")
	middleware.ServeHTTP(httptest.NewRecorder(), req, handler)

	require.Empty(t, received, "active workspace must not be set when header config is empty")
}

func TestWorkspaceActiveMiddleware_HeaderPresent(t *testing.T) {
	config.PrestConf.WorkspaceActiveHeader = "X-Workspace-Id"
	defer func() { config.PrestConf.WorkspaceActiveHeader = "" }()

	middleware := WorkspaceActiveMiddleware()

	var received string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(pctx.WorkspaceIDActiveKey).(string); ok {
			received = v
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Workspace-Id", "ws-7")
	middleware.ServeHTTP(httptest.NewRecorder(), req, handler)

	require.Equal(t, "ws-7", received, "active workspace id should be threaded into context")
}

func TestWorkspaceActiveMiddleware_EmptyHeaderPersonalMode(t *testing.T) {
	config.PrestConf.WorkspaceActiveHeader = "X-Workspace-Id"
	defer func() { config.PrestConf.WorkspaceActiveHeader = "" }()

	middleware := WorkspaceActiveMiddleware()

	var received string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v, ok := r.Context().Value(pctx.WorkspaceIDActiveKey).(string); ok {
			received = v
		}
		w.WriteHeader(http.StatusOK)
	})

	// No header at all → personal mode (key not set).
	req := httptest.NewRequest("GET", "/test", nil)
	middleware.ServeHTTP(httptest.NewRecorder(), req, handler)
	require.Empty(t, received, "absent header means personal mode; key must be unset")
}
