package keto

import (
	"context"
	"errors"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNew_ClientDisabled(t *testing.T) {
	c := New("", "")
	if c.Enabled() {
		t.Fatal("expected disabled client")
	}

	allowed, err := c.Check(context.Background(), "workspace", "ws1", "view", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true when disabled (fail-open)")
	}

	ws, err := c.ListWorkspacesForUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ws) != 0 {
		t.Fatalf("expected empty list when disabled, got %v", ws)
	}
}

func TestCheck_Allowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/relation-tuples/check" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	allowed, err := c.Check(context.Background(), "workspace", "ws1", "view", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true")
	}
}

func TestCheck_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"allowed": false})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	allowed, err := c.Check(context.Background(), "workspace", "ws1", "view", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected allowed=false")
	}
}

func TestCheck_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("keto is down"))
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	allowed, err := c.Check(context.Background(), "workspace", "ws1", "view", "u1")
	if !errors.Is(err, ErrKetoUnhealthy) {
		t.Fatalf("expected ErrKetoUnhealthy, got err=%v allowed=%v", err, allowed)
	}
}

func TestCheckWorkspace(t *testing.T) {
	var got struct {
		Namespace string `json:"namespace"`
		Object    string `json:"object"`
		Relation  string `json:"relation"`
		SubjectID string `json:"subject_id"`
	}
	mu := sync.Mutex{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&got)
		mu.Lock()
		defer mu.Unlock()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	allowed, err := c.CheckWorkspace(context.Background(), "ws-abc", "user-123", PermissionView)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed=true")
	}

	mu.Lock()
	defer mu.Unlock()
	if got.Namespace != "workspace" {
		t.Fatalf("expected namespace=workspace, got %s", got.Namespace)
	}
	if got.Object != "ws-abc" {
		t.Fatalf("expected object=ws-abc, got %s", got.Object)
	}
	if got.Relation != "view" {
		t.Fatalf("expected relation=view, got %s", got.Relation)
	}
	if got.SubjectID != "user-123" {
		t.Fatalf("expected subject_id=user-123, got %s", got.SubjectID)
	}
}

func TestCheck_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := New(srv.URL, srv.URL)
	_, err := c.Check(ctx, "workspace", "ws1", "view", "u1")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestListWorkspacesForUser_EmptyUserID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler for empty userID")
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	ws, err := c.ListWorkspacesForUser(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ws) != 0 {
		t.Fatalf("expected empty, got %v", ws)
	}
}

func TestListWorkspacesForUser_Dedup(t *testing.T) {
	// Returns same workspace object across owners, members, viewers
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keto returns relation_tuples array
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"relation_tuples": []map[string]string{
				{"namespace": "workspace", "object": "ws-1", "relation": r.URL.Query().Get("relation"), "subject_id": "u1"},
				{"namespace": "workspace", "object": "ws-2", "relation": r.URL.Query().Get("relation"), "subject_id": "u1"},
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	ws, err := c.ListWorkspacesForUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ws-1 appears in owners + members + viewers → deduped to one entry
	// ws-2 likewise
	if len(ws) != 2 {
		t.Fatalf("expected 2 unique workspaces, got %d: %v", len(ws), ws)
	}
	seen := map[string]bool{}
	for _, w := range ws {
		if seen[w] {
			t.Fatalf("duplicate workspace: %s", w)
		}
		seen[w] = true
	}
}

func TestListWorkspacesForUser_Pagination(t *testing.T) {
	callCount := 0
	mu := sync.Mutex{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		token := r.URL.Query().Get("page_token")
		switch token {
		case "":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"relation_tuples": []map[string]string{
					{"namespace": "workspace", "object": "ws-1", "relation": "members", "subject_id": "u1"},
				},
				"next_page_token": "tok-page2",
			})
		case "tok-page2":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"relation_tuples": []map[string]string{
					{"namespace": "workspace", "object": "ws-2", "relation": "members", "subject_id": "u1"},
				},
				"next_page_token": "", // last page
			})
		default:
			t.Fatalf("unexpected page_token: %s", token)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	ws, err := c.ListWorkspacesForUser(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ws) != 2 {
		t.Fatalf("expected 2 workspaces, got %d: %v", len(ws), ws)
	}
	// Each relation (owners/members/viewers) makes its own pages;
	// we only set up responses for one relation, so expect at least
	// 1 call per relation (3 total) plus 1 for the second page of
	// the relation that has pagination.
	if callCount < 2 {
		t.Fatalf("expected at least 2 calls, got %d", callCount)
	}
}

func TestListWorkspacesForUser_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	_, err := c.ListWorkspacesForUser(context.Background(), "u1")
	if err == nil {
		t.Fatal("expected error on server error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status 500 in error, got: %v", err)
	}
}

func TestListWorkspacesForUser_PaginationCap(t *testing.T) {
	// Infinite pagination — client should cap at maxPages (10) per relation.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"relation_tuples":  []map[string]string{},
			"next_page_token":  "next",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ws, err := c.ListWorkspacesForUser(ctx, "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ws) != 0 {
		t.Fatalf("expected empty, got %d", len(ws))
	}
}

func TestListWorkspacesForUser_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach handler")
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := New(srv.URL, srv.URL)
	_, err := c.ListWorkspacesForUser(ctx, "u1")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
