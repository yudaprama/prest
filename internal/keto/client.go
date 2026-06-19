// Package keto provides pREST's thin client for Ory Keto's relationship
// API. It is intentionally a separate, dependency-light implementation
// so pREST does not pull in `egent-lobehub/authz` (different module,
// pulls in more deps). The wire format is identical: Keto's v0.12
// relation-tuples read/write API.
//
// pREST uses this client for two purposes:
//
//   1. Single-workspace authorization gate (WorkspaceAuthzGate):
//      when `?workspaceId=…` is present, call `Check` once before the
//      SQL template runs. Fail-open on Keto errors (warn log) to match
//      the TS BFF's `rbacPermission.ts` policy.
//
//   2. Cross-workspace membership resolution (WorkspaceMembershipResolver):
//      call `ListWorkspacesForUser` once per request (cached 30s) to
//      drive the Tier 1 `WHERE workspace_id IN (...)` auto-filter on
//      the four workspace tables and the `workspaceScopeIn` template
//      helper. Fail-closed for workspace tables, fail-open for
//      personal-scope tables.
package keto

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Errors returned by the client. Callers may use errors.Is to decide
// whether to fail-open or fail-closed.
var (
	ErrNotConfigured = errors.New("keto: not configured")
	ErrKetoUnhealthy = errors.New("keto: request failed")
)

// Relation values the workspace namespace understands.
const (
	RelationOwner  = "owners"
	RelationMember = "members"
	RelationViewer = "viewers"

	// Permission tiers in Keto's namespace config:
	//   view   ← viewer+member+owner
	//   write  ← member+owner
	//   manage ← owner
	PermissionView   = "view"
	PermissionWrite  = "write"
	PermissionManage = "manage"

	WorkspaceNamespace = "workspace"
)

// Client is a minimal Keto v0.12 REST client.
type Client struct {
	readURL    string
	writeURL   string
	httpClient *http.Client
}

// New returns a Keto client. Empty readURL disables the client
// (Check returns true, ListWorkspacesForUser returns nil) — used for
// personal-scope deployments or local dev without Keto.
func New(readURL, writeURL string) *Client {
	return &Client{
		readURL:  strings.TrimRight(readURL, "/"),
		writeURL: strings.TrimRight(writeURL, "/"),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Enabled reports whether the client has a read URL configured.
func (c *Client) Enabled() bool {
	return c.readURL != ""
}

// Check answers: does `subjectID` have `relation` on `namespace:object`?
//
// Returns (true, nil) when the client is disabled (fail-open for
// personal scope). Returns (false, err) on transport or non-200
// responses — callers decide whether to fail-open or fail-closed.
func (c *Client) Check(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
	if !c.Enabled() {
		return true, nil
	}

	body := map[string]string{
		"namespace":  namespace,
		"object":     object,
		"relation":   relation,
		"subject_id": subjectID,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("keto: marshal check body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.readURL+"/relation-tuples/check", bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("keto: build check request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrKetoUnhealthy, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("%w: status %d: %s", ErrKetoUnhealthy, resp.StatusCode, respBody)
	}

	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("keto: decode check response: %w", err)
	}
	return result.Allowed, nil
}

// CheckWorkspace is a convenience wrapper for the workspace namespace.
// relation should be PermissionView, PermissionWrite, or PermissionManage.
func (c *Client) CheckWorkspace(ctx context.Context, workspaceID, userID, relation string) (bool, error) {
	return c.Check(ctx, WorkspaceNamespace, workspaceID, relation, userID)
}

// tupleJSON matches Keto's relation-tuple shape.
type tupleJSON struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
	SubjectID string `json:"subject_id"`
}

// ListWorkspacesForUser returns the deduplicated set of workspace
// object IDs that `userID` is a member of (owners ∪ members ∪ viewers).
// Handles Keto pagination (page_tokens) up to maxPages; beyond that
// returns what it has. Empty (not nil) when client disabled.
//
// On transport or HTTP error returns (partial-or-empty, err). The
// resolver middleware decides fail-open vs fail-closed.
func (c *Client) ListWorkspacesForUser(ctx context.Context, userID string) ([]string, error) {
	if !c.Enabled() {
		return []string{}, nil
	}
	if userID == "" {
		return []string{}, nil
	}

	const maxPages = 10
	seen := make(map[string]struct{})
	result := make([]string, 0)

	for _, rel := range []string{RelationOwner, RelationMember, RelationViewer} {
		pageToken := ""
		pages := 0
		for {
			pages++
			if pages > maxPages {
				break
			}

			params := url.Values{}
			params.Set("namespace", WorkspaceNamespace)
			params.Set("relation", rel)
			params.Set("subject_id", userID)
			if pageToken != "" {
				params.Set("page_token", pageToken)
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet,
				c.readURL+"/relation-tuples?"+params.Encode(), nil)
			if err != nil {
				return result, fmt.Errorf("keto: build list request: %w", err)
			}

			resp, err := c.httpClient.Do(req)
			if err != nil {
				return result, fmt.Errorf("%w: %v", ErrKetoUnhealthy, err)
			}

			if resp.StatusCode != http.StatusOK {
				respBody, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				return result, fmt.Errorf("%w: status %d: %s", ErrKetoUnhealthy, resp.StatusCode, respBody)
			}

			var page struct {
				Tuples []tupleJSON `json:"relation_tuples"`
				// Keto v0.12 uses `next_page_token` (may be empty on last page).
				NextPageToken string `json:"next_page_token"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
				resp.Body.Close()
				return result, fmt.Errorf("keto: decode list response: %w", err)
			}
			resp.Body.Close()

			for _, t := range page.Tuples {
				if t.Object == "" {
					continue
				}
				if _, ok := seen[t.Object]; ok {
					continue
				}
				seen[t.Object] = struct{}{}
				result = append(result, t.Object)
			}

			pageToken = page.NextPageToken
			if pageToken == "" {
				break
			}
		}
	}

	return result, nil
}
