package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/stretchr/testify/require"
	"github.com/urfave/negroni/v3"
)

func TestUserFilterMiddleware_NoConfig(t *testing.T) {
	// Reset config
	config.PrestConf.UserIDHeader = ""
	config.PrestConf.UserIDFilters = nil

	// Create middleware
	middleware := UserFilterMiddleware()

	// Create test handler that checks context
	var receivedUserID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := r.Context().Value(pctx.UserIDKey).(string); ok {
			receivedUserID = userID
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create request with header (should be ignored since config is empty)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-Id", "user-123")
	rw := httptest.NewRecorder()

	middleware.ServeHTTP(rw, req, handler)

	require.Empty(t, receivedUserID, "UserID should not be set when config is empty")
	require.Equal(t, http.StatusOK, rw.Code)
}

func TestUserFilterMiddleware_WithConfig(t *testing.T) {
	// Set config
	config.PrestConf.UserIDHeader = "X-User-Id"
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{}

	// Create middleware
	middleware := UserFilterMiddleware()

	// Create test handler that checks context
	var receivedUserID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := r.Context().Value(pctx.UserIDKey).(string); ok {
			receivedUserID = userID
		}
		w.WriteHeader(http.StatusOK)
	})

	// Test 1: Request with header
	t.Run("with header", func(t *testing.T) {
		receivedUserID = ""
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-Id", "user-123")
		rw := httptest.NewRecorder()

		middleware.ServeHTTP(rw, req, handler)

		require.Equal(t, "user-123", receivedUserID)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	// Test 2: Request without header
	t.Run("without header", func(t *testing.T) {
		receivedUserID = ""
		req := httptest.NewRequest("GET", "/test", nil)
		rw := httptest.NewRecorder()

		middleware.ServeHTTP(rw, req, handler)

		require.Empty(t, receivedUserID)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	// Test 3: Request with empty header value
	t.Run("with empty header value", func(t *testing.T) {
		receivedUserID = ""
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-Id", "")
		rw := httptest.NewRecorder()

		middleware.ServeHTTP(rw, req, handler)

		require.Empty(t, receivedUserID)
		require.Equal(t, http.StatusOK, rw.Code)
	})

	// Test 4: Request with different header name
	t.Run("with different header name", func(t *testing.T) {
		receivedUserID = ""
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Auth-User", "user-456")
		rw := httptest.NewRecorder()

		middleware.ServeHTTP(rw, req, handler)

		require.Empty(t, receivedUserID, "Should not extract from wrong header")
		require.Equal(t, http.StatusOK, rw.Code)
	})
}

func TestUserFilterMiddleware_CustomHeaderName(t *testing.T) {
	// Set config with custom header name
	config.PrestConf.UserIDHeader = "X-Auth-UserId"
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{}

	middleware := UserFilterMiddleware()

	var receivedUserID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := r.Context().Value(pctx.UserIDKey).(string); ok {
			receivedUserID = userID
		}
		w.WriteHeader(http.StatusOK)
	})

	// Test with custom header
	receivedUserID = ""
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-UserId", "custom-user-789")
	rw := httptest.NewRecorder()

	middleware.ServeHTTP(rw, req, handler)

	require.Equal(t, "custom-user-789", receivedUserID)
	require.Equal(t, http.StatusOK, rw.Code)
}

func TestUserFilterMiddleware_ContextPropagation(t *testing.T) {
	config.PrestConf.UserIDHeader = "X-User-Id"
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{}

	middleware := UserFilterMiddleware()

	// Chain of handlers to verify context propagation
	var contextValues []string
	firstHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if userID, ok := ctx.Value(pctx.UserIDKey).(string); ok {
			contextValues = append(contextValues, "first:"+userID)
		}
	})

	// Verify context is passed to next handler
	t.Run("context propagates to next handler", func(t *testing.T) {
		contextValues = nil
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-Id", "propagated-user")
		rw := httptest.NewRecorder()

		middleware.ServeHTTP(rw, req, firstHandler)

		require.Contains(t, contextValues, "first:propagated-user")
	})
}

func TestUserFilterMiddleware_MultipleRequests(t *testing.T) {
	config.PrestConf.UserIDHeader = "X-User-Id"
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{}

	middleware := UserFilterMiddleware()

	var receivedUserID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := r.Context().Value(pctx.UserIDKey).(string); ok {
			receivedUserID = userID
		}
		w.WriteHeader(http.StatusOK)
	})

	// Process multiple requests with different user IDs
	userIDs := []string{"user-1", "user-2", "user-3", "user-4", "user-5"}

	for _, expectedUserID := range userIDs {
		receivedUserID = ""
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-User-Id", expectedUserID)
		rw := httptest.NewRecorder()

		middleware.ServeHTTP(rw, req, handler)

		require.Equal(t, expectedUserID, receivedUserID, "Each request should get its own user ID")
		require.Equal(t, http.StatusOK, rw.Code)
	}
}

func TestUserFilterMiddleware_ContextKeyTypeSafe(t *testing.T) {
	config.PrestConf.UserIDHeader = "X-User-Id"
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{}

	middleware := UserFilterMiddleware()

	var typeCheckPassed bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// Verify it's a string, not some other type
		if _, ok := ctx.Value(pctx.UserIDKey).(string); ok {
			typeCheckPassed = true
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-User-Id", "type-test-user")
	rw := httptest.NewRecorder()

	middleware.ServeHTTP(rw, req, handler)

	require.True(t, typeCheckPassed, "Context value should be type string")
}

func TestUserFilterMiddleware_NegroniIntegration(t *testing.T) {
	config.PrestConf.UserIDHeader = "X-User-Id"
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{}

	var receivedUserID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := r.Context().Value(pctx.UserIDKey).(string); ok {
			receivedUserID = userID
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	n := negroni.New()
	n.Use(UserFilterMiddleware())
	n.UseHandler(handler)

	server := httptest.NewServer(n)
	defer server.Close()

	// Test request
	req, err := http.NewRequest("GET", server.URL+"/test", nil)
	require.NoError(t, err)
	req.Header.Set("X-User-Id", "integration-user")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "integration-user", receivedUserID)
}
