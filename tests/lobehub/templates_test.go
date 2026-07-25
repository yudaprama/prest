// Package lobehubtest holds unit tests for the LobeHub migration
// additions in pREST that do not require a live database connection.
//
// They live outside `adapters/postgres` so they do not trigger the
// package's `init()` (which calls `adapters/postgres.Load()` and
// `os.Exit(1)` on connection failure).
package lobehubtest

import (
	"os"
	"path/filepath"
	gotemplate "text/template"
	"strings"
	"testing"

	"github.com/prest/prest/v2/template"
)

// scope constants for testTemplateData.
const (
	scopePersonal        = "personal"
	scopeSingleTenant = "single_tenant"
)

// TestLobehubTemplatesParse validates every `.read.sql` under
// prest/etc/queries/lobehub through the same Go text/template engine
// that ParseScript uses, without requiring a live Postgres connection.
//
// It runs each template twice (personal-mode + tenant-mode) and
// asserts:
//   1. Template parses without syntax error
//   2. Template executes without panic/error
//   3. Output SQL contains at least one statement
func TestLobehubTemplatesParse(t *testing.T) {
	scripts := discoverScripts(t)
	for _, name := range scripts {
		name := name
		t.Run("personal/"+name, func(t *testing.T) {
			executeLobehubScript(t, name, testTemplateData(scopePersonal))
		})
		t.Run("tenant/"+name, func(t *testing.T) {
			executeLobehubScript(t, name, testTemplateData(scopeSingleTenant))
		})

	}
}

func discoverScripts(t *testing.T) []string {
	t.Helper()
	queriesDir := resolveQueriesDir(t)
	entries, err := os.ReadDir(queriesDir)
	if err != nil {
		t.Fatalf("read queries dir: %v", err)
	}
	var scripts []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".read.sql") {
			scripts = append(scripts, e.Name())
		}
	}
	if len(scripts) == 0 {
		t.Fatal("no .read.sql scripts found")
	}
	return scripts
}

func resolveQueriesDir(t *testing.T) string {
	t.Helper()
	for _, d := range []string{
		filepath.Join("..", "..", "etc", "queries", "lobehub"),
		filepath.Join("..", "..", "..", "etc", "queries", "lobehub"),
	} {
		abs := filepath.Clean(d)
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			return abs
		}
	}
	wd, _ := os.Getwd()
	t.Fatalf("cannot find etc/queries/lobehub relative to %s (tried 2 variants)", wd)
	return ""
}

func executeLobehubScript(t *testing.T, name string, data map[string]interface{}) {
	t.Helper()
	queriesDir := resolveQueriesDir(t)
	scriptPath := filepath.Join(queriesDir, name)

	funcRegistry := &template.FuncRegistry{TemplateData: data}
	tpl := gotemplate.New(name).Funcs(funcRegistry.RegistryAllFuncs())

	tpl, err := tpl.ParseFiles(scriptPath)
	if err != nil {
		t.Fatalf("template parse error: %v", err)
	}

	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute error: %v", err)
	}

	sql := buf.String()
	if strings.TrimSpace(sql) == "" {
		t.Fatal("executed SQL is empty")
	}
	if !strings.Contains(strings.ToUpper(sql), "SELECT") {
		t.Logf("WARNING: SQL does not contain SELECT — %s", sql[:min(len(sql), 80)])
	}
}

// TestLobehubTenantBranching verifies the tenant_id IS NULL leak
// fix in the tenant-bearing scripts: personal-mode SQL must contain
// "tenant_id IS NULL", tenant-mode SQL must not.
func TestLobehubTenantBranching(t *testing.T) {
	tenantScripts := map[string]bool{
		"sessionsListGrouped.read.sql":              true,
		"topicsListBySession.read.sql":              true,
		"messagesListByTopic.read.sql":              true,
		"agentsListWithStats.read.sql":              true,
		"usageAggregateByDay.read.sql":              true,
		"threadMessages.read.sql":                   true,
		"agentFilesByAgent.read.sql":                true,
		"connectorToolsByConnector.read.sql":        true,
		"verifyResultsWithRubric.read.sql":          true,
		"generationBatchesWithGenerations.read.sql": true,
		"knowledgeBaseFilesWithChunks.read.sql":     true,
		"agentSkillsWithResources.read.sql":         true,
	}
	// notificationsListWithDeliveries is the only one without a tenant_id column.
	personalOnly := map[string]bool{
		"notificationsListWithDeliveries.read.sql": true,
	}

	for name := range tenantScripts {
		name := name
		t.Run(name+"_personal_has_tenant_null", func(t *testing.T) {
			sql := renderScript(t, name, scopePersonal)
			if !strings.Contains(sql, "tenant_id IS NULL") {
				t.Errorf("personal-mode SQL missing 'tenant_id IS NULL' clause\nSQL:\n%s", sql)
			}
		})
		t.Run(name+"_tenant_does_not_branch_to_null", func(t *testing.T) {
			sql := renderScript(t, name, scopeSingleTenant)
			// The tenant-mode branch should produce `tenant_id = $N`, not
			// `tenant_id IS NULL` (which is the personal-mode branch).
			// The join `AND g.tenant_id = $N` style is the tenant branch.
			if strings.Contains(sql, "AND g.tenant_id IS NULL") {
				t.Errorf("tenant-mode SQL incorrectly contains 'AND g.tenant_id IS NULL'\nSQL:\n%s", sql)
			}
			// Outer WHERE must use tenant_id = $N (not IS NULL)
			if strings.Contains(sql, "WHERE  s.tenant_id IS NULL") {
				t.Errorf("tenant-mode SQL contains 'WHERE s.tenant_id IS NULL' (personal branch)\nSQL:\n%s", sql)
			}
			if !strings.Contains(sql, "tenant_id = $") {
				t.Errorf("tenant-mode SQL missing 'tenant_id = $' predicate\nSQL:\n%s", sql)
			}
		})
	}
	for name := range personalOnly {
		name := name
		t.Run(name+"_no_tenant_branching", func(t *testing.T) {
			personalSQL := renderScript(t, name, scopePersonal)
			tenantSQL := renderScript(t, name, scopeSingleTenant)
			// Personal-only template does not branch on tenantId
			if strings.Contains(personalSQL, "tenant_id IS NULL") {
				t.Errorf("personal-only template unexpectedly contains 'tenant_id IS NULL'\nSQL:\n%s", personalSQL)
			}
			if strings.Contains(tenantSQL, "tenant_id") {
				t.Errorf("personal-only template unexpectedly references tenant_id\nSQL:\n%s", tenantSQL)
			}
		})
	}
}

func renderScript(t *testing.T, name string, scope string) string {
	t.Helper()
	queriesDir := resolveQueriesDir(t)
	scriptPath := filepath.Join(queriesDir, name)

	data := testTemplateData(scope)
	funcRegistry := &template.FuncRegistry{TemplateData: data}
	tpl := gotemplate.New(name).Funcs(funcRegistry.RegistryAllFuncs())
	tpl, err := tpl.ParseFiles(scriptPath)
	if err != nil {
		t.Fatalf("template parse error: %v", err)
	}
	var buf strings.Builder
	if err := tpl.Execute(&buf, data); err != nil {
		t.Fatalf("template execute error: %v", err)
	}
	return buf.String()
}

// testTemplateData returns representative values for every parameter any
// lobehub script references. `scope` selects which branch of the
// tenant_id / tenantScope / tenantIds template helpers to
// exercise:
//
//   - scopePersonal:        user_id filter only
//   - scopeSingleTenant: ?tenantId=X path (single tenant)
func testTemplateData(scope string) map[string]interface{} {
	data := map[string]interface{}{
		"userId":          "00000000-0000-0000-0000-000000000001",
		"sessionId":       "sess-1",
		"topicId":         "topic-1",
		"sessionGroupId":  "sg-1",
		"groupId":         "group-1",
		"agentId":         "agent-1",
		"operationId":     "op-1",
		"userConnectorId": "00000000-0000-0000-0000-000000000099",
		"knowledgeBaseId": "kb-1",
		"source":          "user",
		"category":        "system",
		"type":            "info",
		"status":          "active",
		"keyword":         "alpha",
		"keywordPattern":  "%alpha%",
		"unreadOnly":      "true",
		"activeOnly":      "true",
		"includePinnedOnly": "true",
		"pinnedOnly":      "true",
		"favorite":        "true",
		"includeDisabled": "true",
		"includeContent":  "true",
		"model":           "test/model",
		"provider":        "test",
		"tagList":         "a,b",
		"startDate":       "2024-01-01T00:00:00Z",
		"endDate":         "2024-12-31T23:59:59Z",
		"page":            "1",
		"size":            "20",
	}
	if scope == scopeSingleTenant {
		data["tenantId"] = "00000000-0000-0000-0000-00000000aabb"
	}
	return data
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
