// Package middlewarestest holds unit tests for the pREST middleware
// additions that don't require a live database. It is kept outside the
// `middlewares` package so it doesn't trigger the package's `init()`
// in `config_test.go` (which calls `postgres.Load()` and `os.Exit(1)`
// on connection failure).
package middlewarestest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/prest/prest/v2/internal/keto"
	"github.com/prest/prest/v2/middlewares"
	"github.com/urfave/negroni/v3"
)

func init() {
	if config.PrestConf == nil {
		config.PrestConf = &config.Prest{}
	}
}

// runMiddleware runs the given negroni handler against a test request
// and returns the final http.ResponseWriter and the request that reached
// the inner handler (if any). If the middleware short-circuits (403,
// 500, etc.), the inner handler is never called and the test inspects
// the status code on the recorder.
func runMiddleware(t *testing.T, mw negroni.Handler, r *http.Request) (int, *http.Request) {
	t.Helper()
	var captured *http.Request
	rec := httptest.NewRecorder()

	n := negroni.New()
	n.Use(mw)
	n.UseHandler(http.HandlerFunc(func(_ http.ResponseWriter, inner *http.Request) {
		captured = inner
	}))
	n.ServeHTTP(rec, r)
	return rec.Code, captured
}

// --- WorkspaceAuthzGate ---

func TestWorkspaceAuthzGate_DisabledFlag(t *testing.T) {
	config.PrestConf.KetoEnabled = false
	config.PrestConf.KetoReadURL = ""
	defer func() {
		config.PrestConf.KetoEnabled = false
		config.PrestConf.KetoReadURL = ""
	}()

	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped?workspaceId=ws-x", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, captured := runMiddleware(t, middlewares.WorkspaceAuthzGate(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (gate disabled), got %d", code)
	}
	if captured == nil {
		t.Fatal("expected inner handler to be called")
	}
	// Gate disabled → WorkspaceIDKey should NOT be set
	if got, ok := captured.Context().Value(pctx.WorkspaceIDKey).(string); ok && got != "" {
		t.Fatalf("expected WorkspaceIDKey to be empty when gate disabled, got %q", got)
	}
}

func TestWorkspaceAuthzGate_NoWorkspaceID_PassThrough(t *testing.T) {
	config.PrestConf.KetoEnabled = true
	config.PrestConf.KetoReadURL = "http://keto.example.invalid"
	config.PrestConf.KetoWriteURL = "http://keto.example.invalid"
	defer func() {
		config.PrestConf.KetoEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	// No ?workspaceId — gate is skipped
	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, captured := runMiddleware(t, middlewares.WorkspaceAuthzGate(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (no workspaceId), got %d", code)
	}
	if captured == nil {
		t.Fatal("expected inner handler called")
	}
}

func TestWorkspaceAuthzGate_NoIdentity_PassThrough(t *testing.T) {
	config.PrestConf.KetoEnabled = true
	config.PrestConf.KetoReadURL = "http://keto.example.invalid"
	config.PrestConf.KetoWriteURL = "http://keto.example.invalid"
	defer func() {
		config.PrestConf.KetoEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	// workspaceId present but no UserIDKey → pass-through (personal-scope BFF callers)
	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped?workspaceId=ws-x", nil)

	code, captured := runMiddleware(t, middlewares.WorkspaceAuthzGate(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (no identity), got %d", code)
	}
	if captured == nil {
		t.Fatal("expected inner handler called")
	}
}

func TestWorkspaceAuthzGate_Allowed(t *testing.T) {
	// Mock Keto that always allows.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	config.PrestConf.KetoEnabled = true
	config.PrestConf.KetoReadURL = srv.URL
	config.PrestConf.KetoWriteURL = srv.URL
	defer func() {
		config.PrestConf.KetoEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped?workspaceId=ws-x", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, captured := runMiddleware(t, middlewares.WorkspaceAuthzGate(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (allowed), got %d", code)
	}
	if captured == nil {
		t.Fatal("expected inner handler called")
	}
	got, ok := captured.Context().Value(pctx.WorkspaceIDKey).(string)
	if !ok || got != "ws-x" {
		t.Fatalf("expected WorkspaceIDKey=ws-x, got %q", got)
	}
}

func TestWorkspaceAuthzGate_Denied_Returns403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"allowed": false})
	}))
	defer srv.Close()

	config.PrestConf.KetoEnabled = true
	config.PrestConf.KetoReadURL = srv.URL
	config.PrestConf.KetoWriteURL = srv.URL
	defer func() {
		config.PrestConf.KetoEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped?workspaceId=ws-x", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, _ := runMiddleware(t, middlewares.WorkspaceAuthzGate(), req)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 (denied), got %d", code)
	}
}

func TestWorkspaceAuthzGate_KetoUnreachable_FailOpen(t *testing.T) {
	// Point to a port with no listener — request fails fast.
	config.PrestConf.KetoEnabled = true
	config.PrestConf.KetoReadURL = "http://127.0.0.1:1"
	config.PrestConf.KetoWriteURL = "http://127.0.0.1:1"
	defer func() {
		config.PrestConf.KetoEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped?workspaceId=ws-x", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, captured := runMiddleware(t, middlewares.WorkspaceAuthzGate(), req)
	// Fail-open: 200 (NOT 403 or 503)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open), got %d", code)
	}
	if captured == nil {
		t.Fatal("expected inner handler called (fail-open)")
	}
	// WorkspaceIDKey still set so template can use it
	if got, _ := captured.Context().Value(pctx.WorkspaceIDKey).(string); got != "ws-x" {
		t.Fatalf("expected WorkspaceIDKey=ws-x on fail-open, got %q", got)
	}
}

func TestWorkspaceAuthzGate_Keto500_FailOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "keto error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	config.PrestConf.KetoEnabled = true
	config.PrestConf.KetoReadURL = srv.URL
	config.PrestConf.KetoWriteURL = srv.URL
	defer func() {
		config.PrestConf.KetoEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped?workspaceId=ws-x", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, _ := runMiddleware(t, middlewares.WorkspaceAuthzGate(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open on 500), got %d", code)
	}
}

// --- WorkspaceMembershipResolver ---

func TestWorkspaceMembershipResolver_DisabledFlag(t *testing.T) {
	config.PrestConf.WorkspaceFiltersEnabled = false
	config.PrestConf.KetoReadURL = ""
	defer func() {
		config.PrestConf.WorkspaceFiltersEnabled = false
		config.PrestConf.KetoReadURL = ""
	}()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, captured := runMiddleware(t, middlewares.WorkspaceMembershipResolver(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (resolver disabled), got %d", code)
	}
	if captured == nil {
		t.Fatal("expected inner handler called")
	}
	// Disabled → WorkspaceIDsKey should NOT be set
	if got, ok := captured.Context().Value(pctx.WorkspaceIDsKey).([]string); ok {
		t.Fatalf("expected WorkspaceIDsKey to be nil when disabled, got %v", got)
	}
}

func TestWorkspaceMembershipResolver_NoIdentity_EmptyList(t *testing.T) {
	config.PrestConf.WorkspaceFiltersEnabled = true
	defer func() { config.PrestConf.WorkspaceFiltersEnabled = false }()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	// No UserIDKey set

	code, captured := runMiddleware(t, middlewares.WorkspaceMembershipResolver(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (no identity), got %d", code)
	}
	if captured == nil {
		t.Fatal("expected inner handler called")
	}
	got, ok := captured.Context().Value(pctx.WorkspaceIDsKey).([]string)
	if !ok {
		t.Fatal("expected WorkspaceIDsKey to be set (empty list)")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %v", got)
	}
}

func TestWorkspaceMembershipResolver_KetoUnreachable_EmptyList(t *testing.T) {
	config.PrestConf.WorkspaceFiltersEnabled = true
	config.PrestConf.KetoReadURL = "http://127.0.0.1:1"
	config.PrestConf.KetoWriteURL = "http://127.0.0.1:1"
	defer func() {
		config.PrestConf.WorkspaceFiltersEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, captured := runMiddleware(t, middlewares.WorkspaceMembershipResolver(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (fail-open for non-workspace tables), got %d", code)
	}
	got, ok := captured.Context().Value(pctx.WorkspaceIDsKey).([]string)
	if !ok {
		t.Fatal("expected WorkspaceIDsKey to be set even on Keto error")
	}
	// Fail-closed for workspace tables: empty list means `WHERE col IN ()` → FALSE.
	if len(got) != 0 {
		t.Fatalf("expected empty list on Keto error, got %v", got)
	}
}

func TestWorkspaceMembershipResolver_Resolved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return 1 workspace per relation (owners/members/viewers) — 3 total after dedup
		// but each relation returns a different workspace.
		rel := r.URL.Query().Get("relation")
		var ws string
		switch rel {
		case "owners":
			ws = "ws-owner"
		case "members":
			ws = "ws-member"
		case "viewers":
			ws = "ws-viewer"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"relation_tuples": []map[string]string{
				{"namespace": "workspace", "object": ws, "relation": rel, "subject_id": "u1"},
			},
		})
	}))
	defer srv.Close()

	config.PrestConf.WorkspaceFiltersEnabled = true
	config.PrestConf.KetoReadURL = srv.URL
	config.PrestConf.KetoWriteURL = srv.URL
	defer func() {
		config.PrestConf.WorkspaceFiltersEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	// Clear the cache so the test sees fresh values.
	middlewares.ResetMembershipCacheForTest()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "u1"))

	code, captured := runMiddleware(t, middlewares.WorkspaceMembershipResolver(), req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	got, ok := captured.Context().Value(pctx.WorkspaceIDsKey).([]string)
	if !ok {
		t.Fatal("expected WorkspaceIDsKey to be set")
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 workspaces, got %d: %v", len(got), got)
	}
	seen := map[string]bool{}
	for _, w := range got {
		seen[w] = true
	}
	if !seen["ws-owner"] || !seen["ws-member"] || !seen["ws-viewer"] {
		t.Fatalf("expected [ws-owner, ws-member, ws-viewer], got %v", got)
	}
}

func TestWorkspaceMembershipResolver_CacheHit(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"relation_tuples": []map[string]string{
				{"namespace": "workspace", "object": "ws-1", "relation": "members", "subject_id": "u1"},
			},
		})
	}))
	defer srv.Close()

	config.PrestConf.WorkspaceFiltersEnabled = true
	config.PrestConf.KetoReadURL = srv.URL
	config.PrestConf.KetoWriteURL = srv.URL
	defer func() {
		config.PrestConf.WorkspaceFiltersEnabled = false
		config.PrestConf.KetoReadURL = ""
		config.PrestConf.KetoWriteURL = ""
	}()

	middlewares.ResetMembershipCacheForTest()

	// First request hits Keto.
	req1 := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req1 = req1.WithContext(context.WithValue(req1.Context(), pctx.UserIDKey, "u-cache"))
	code1, c1 := runMiddleware(t, middlewares.WorkspaceMembershipResolver(), req1)
	if code1 != http.StatusOK {
		t.Fatalf("expected 200 on first req, got %d", code1)
	}
	firstCallCount := callCount

	// Second request for the same user should hit cache (callCount unchanged).
	req2 := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), pctx.UserIDKey, "u-cache"))
	code2, c2 := runMiddleware(t, middlewares.WorkspaceMembershipResolver(), req2)
	if code2 != http.StatusOK {
		t.Fatalf("expected 200 on second req, got %d", code2)
	}

	if callCount != firstCallCount {
		t.Fatalf("expected cache hit (callCount unchanged at %d), got %d", firstCallCount, callCount)
	}
	_ = c1
	_ = c2
}

// avoid unused-import: keto is imported for the constant reference.
var _ = keto.PermissionView