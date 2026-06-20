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
	gotemplate "text/template"
	"testing"

	"github.com/prest/prest/v2/adapters/postgres"
	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
	"github.com/prest/prest/v2/template"
)

func init() {
	if config.PrestConf == nil {
		config.PrestConf = &config.Prest{}
	}
}

// --- WorkspaceFilterConfig + ResolveWorkspaceIDColumn ---

func TestResolveWorkspaceIDColumn_NoFilters(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = nil

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	column := postgres.ResolveWorkspaceIDColumn(req)
	if column != "" {
		t.Fatalf("expected empty, got %q", column)
	}
}

func TestResolveWorkspaceIDColumn_MatchFound(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "id"},
	}
	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	column := postgres.ResolveWorkspaceIDColumn(req)
	if column != "id" {
		t.Fatalf("expected id, got %q", column)
	}
}

func TestResolveWorkspaceIDColumn_WorkspaceMembers(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "id"},
		{Database: "lobehub", Schema: "public", Table: "workspace_members", Column: "workspace_id"},
	}
	req := httptest.NewRequest("GET", "/lobehub/public/workspace_members", nil)
	column := postgres.ResolveWorkspaceIDColumn(req)
	if column != "workspace_id" {
		t.Fatalf("expected workspace_id, got %q", column)
	}
}

func TestResolveWorkspaceIDColumn_NoMatch(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "id"},
	}
	req := httptest.NewRequest("GET", "/lobehub/public/sessions", nil)
	column := postgres.ResolveWorkspaceIDColumn(req)
	if column != "" {
		t.Fatalf("expected empty (no matching table), got %q", column)
	}
}

func TestWorkspaceIDsFromContext_NotSet(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	got := postgres.WorkspaceIDsFromContext(req)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestWorkspaceIDsFromContext_EmptyList(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDsKey, []string{}))
	got := postgres.WorkspaceIDsFromContext(req)
	if got == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestWorkspaceIDsFromContext_Populated(t *testing.T) {
	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDsKey, []string{"ws-a", "ws-b"}))
	got := postgres.WorkspaceIDsFromContext(req)
	if len(got) != 2 || got[0] != "ws-a" || got[1] != "ws-b" {
		t.Fatalf("expected [ws-a, ws-b], got %v", got)
	}
}

// --- WhereByRequest: workspace IN-clause injection ---

func TestWhereByRequest_WorkspaceFilter_SingleWorkspace(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "id"},
	}
	defer func() { config.PrestConf.WorkspaceIDFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDsKey, []string{"ws-1"}))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, `"id" IN ($1)`) {
		t.Fatalf("expected IN-clause, got where=%q", where)
	}
	if len(values) != 1 || values[0] != "ws-1" {
		t.Fatalf("expected values=[ws-1], got %v", values)
	}
}

func TestWhereByRequest_WorkspaceFilter_MultipleWorkspaces(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspace_members", Column: "workspace_id"},
	}
	defer func() { config.PrestConf.WorkspaceIDFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/workspace_members", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDsKey, []string{"ws-1", "ws-2", "ws-3"}))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, `"workspace_id" IN ($1,$2,$3)`) {
		t.Fatalf("expected IN-clause with 3 placeholders, got where=%q", where)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %v", values)
	}
}

func TestWhereByRequest_WorkspaceFilter_EmptyList_FailClosed(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "id"},
	}
	defer func() { config.PrestConf.WorkspaceIDFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDsKey, []string{}))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, "FALSE") {
		t.Fatalf("expected fail-closed FALSE clause, got where=%q", where)
	}
	if len(values) != 0 {
		t.Fatalf("expected no values for FALSE, got %v", values)
	}
}

func TestWhereByRequest_WorkspaceFilter_NoRuleConfigured(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = nil

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDsKey, []string{"ws-1"}))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(where, "workspace") {
		t.Fatalf("expected no workspace clause (rule not configured), got %q", where)
	}
	if len(values) != 0 {
		t.Fatalf("expected no values, got %v", values)
	}
}

func TestWhereByRequest_WorkspaceFilter_NilContext(t *testing.T) {
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "id"},
	}
	defer func() { config.PrestConf.WorkspaceIDFilters = nil }()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	// No WorkspaceIDsKey set on context.

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil list → no clause injected (matches pre-Phase-2 behaviour for tables
	// that have a rule but the resolver hasn't run yet — eg for unauthenticated
	// requests or when the gate is disabled).
	if strings.Contains(where, "FALSE") || strings.Contains(where, "IN (") {
		t.Fatalf("expected no IN/FALSE clause for nil list, got %q", where)
	}
	if len(values) != 0 {
		t.Fatalf("expected no values, got %v", values)
	}
}

func TestWhereByRequest_UserAndWorkspaceFiltersCombined(t *testing.T) {
	config.PrestConf.UserIDFilters = []config.UserFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "owner_id"},
	}
	config.PrestConf.WorkspaceIDFilters = []config.WorkspaceFilterConfig{
		{Database: "lobehub", Schema: "public", Table: "workspaces", Column: "id"},
	}
	defer func() {
		config.PrestConf.UserIDFilters = nil
		config.PrestConf.WorkspaceIDFilters = nil
	}()

	req := httptest.NewRequest("GET", "/lobehub/public/workspaces", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "user-123"))
	req = req.WithContext(context.WithValue(req.Context(), pctx.WorkspaceIDsKey, []string{"ws-1", "ws-2"}))

	adapter := &postgres.Postgres{}
	where, values, err := adapter.WhereByRequest(req, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(where, `"owner_id" = $1`) {
		t.Fatalf("expected user_id filter, got %q", where)
	}
	if !strings.Contains(where, `"id" IN ($2,$3)`) {
		t.Fatalf("expected workspace IN filter, got %q", where)
	}
	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %v", values)
	}
	if values[0] != "user-123" || values[1] != "ws-1" || values[2] != "ws-2" {
		t.Fatalf("values mismatch: %v", values)
	}
}

// --- workspaceScopeIn template helper ---

func TestWorkspaceScopeIn_WithIDs(t *testing.T) {
	data := map[string]interface{}{
		"workspaceIds": []string{"ws-a", "ws-b", "ws-c"},
	}
	fr := &template.FuncRegistry{TemplateData: data}
	tpl := gotemplate.New("test").Funcs(fr.RegistryAllFuncs())
	tpl, err := tpl.Parse(`SELECT * FROM t WHERE {{ workspaceScopeIn "t.workspace_id" }}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("exec: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"t"."workspace_id" IN ($1,$2,$3)`) {
		t.Fatalf("unexpected SQL: %s", out)
	}
	if len(fr.Args) != 3 || fr.Args[0] != "ws-a" || fr.Args[2] != "ws-c" {
		t.Fatalf("args mismatch: %v", fr.Args)
	}
}

func TestWorkspaceScopeIn_EmptyList(t *testing.T) {
	data := map[string]interface{}{
		"workspaceIds": []string{},
	}
	fr := &template.FuncRegistry{TemplateData: data}
	tpl := gotemplate.New("test").Funcs(fr.RegistryAllFuncs())
	tpl, err := tpl.Parse(`SELECT * FROM t WHERE {{ workspaceScopeIn "t.workspace_id" }}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(buf.String(), "FALSE") {
		t.Fatalf("expected FALSE for empty list, got: %s", buf.String())
	}
}

func TestWorkspaceScopeIn_MissingKey(t *testing.T) {
	data := map[string]interface{}{}
	fr := &template.FuncRegistry{TemplateData: data}
	tpl := gotemplate.New("test").Funcs(fr.RegistryAllFuncs())
	tpl, err := tpl.Parse(`SELECT * FROM t WHERE {{ workspaceScopeIn "t.workspace_id" }}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !strings.Contains(buf.String(), "FALSE") {
		t.Fatalf("expected FALSE for missing key, got: %s", buf.String())
	}
}

func TestWorkspaceScopeIn_InvalidIdentifier(t *testing.T) {
	data := map[string]interface{}{
		"workspaceIds": []string{"ws-a"},
	}
	fr := &template.FuncRegistry{TemplateData: data}
	tpl := gotemplate.New("test").Funcs(fr.RegistryAllFuncs())
	tpl, err := tpl.Parse(`SELECT * FROM t WHERE {{ workspaceScopeIn "1bad-name; DROP TABLE x" }}`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("exec: %v", err)
	}
	// Invalid identifier → FALSE (fail safe)
	if !strings.Contains(buf.String(), "FALSE") {
		t.Fatalf("expected FALSE for invalid identifier, got: %s", buf.String())
	}
}

// The end-to-end workspace template var plumbing is exercised by
// `templates_test.go` via the three-branch scope test (personal /
// single-workspace / cross-workspace).

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