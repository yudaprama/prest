package template

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/template"

	"github.com/prest/prest/v2/internal/ident"
)

// FuncRegistry registry func for templates
type FuncRegistry struct {
	TemplateData map[string]interface{}
	Args         []interface{}
	next         int
}

// RegistryAllFuncs for template
func (fr *FuncRegistry) RegistryAllFuncs() (funcs template.FuncMap) {
	funcs = template.FuncMap{
		"isSet":          fr.isSet,
		"defaultOrValue": fr.defaultOrValue,
		"inFormat":       fr.inFormat,
		"unEscape":       fr.unEscape,
		"split":          fr.split,
		"limitOffset":    fr.limitOffset,
		// secure SQL helpers
		"sqlVal":  fr.sqlVal,
		"sqlList": fr.sqlList,
		"ident":   fr.ident,
		// workspaceScopeIn emits `col IN ($1, $2, …)` for the caller's
		// resolved workspace membership (workspaceIds template var).
		// Returns `FALSE` when no memberships are resolved, so
		// cross-workspace reads return nothing for users with no
		// workspaces. The `col` argument is quoted via internal/ident.
		"workspaceScopeIn": fr.workspaceScopeIn,
	}
	return
}

func (fr *FuncRegistry) isSet(key string) (ok bool) {
	_, ok = fr.TemplateData[key]
	return
}

func (fr *FuncRegistry) defaultOrValue(key, defaultValue string) (value interface{}) {
	if ok := fr.isSet(key); !ok {
		fr.TemplateData[key] = defaultValue
	}
	value = fr.TemplateData[key]
	return
}

func (fr *FuncRegistry) inFormat(key string) (query string) {
	items, ok := fr.TemplateData[key].([]string)
	if !ok {
		query = fmt.Sprintf("('%v')", fr.TemplateData[key])
		return
	}
	query = fmt.Sprintf("('%s')", strings.Join(items, "', '"))
	return
}

func (fr *FuncRegistry) unEscape(key string) (value string) {
	value, _ = url.QueryUnescape(key)
	return
}

func (fr *FuncRegistry) split(orig, sep string) (values []string) {
	values = strings.Split(orig, sep)
	return
}

// LimitOffset create and format limit query (offset, SQL ANSI)
func LimitOffset(pageNumberStr, pageSizeStr string) (paginatedQuery string, err error) {
	pageNumber, err := strconv.Atoi(pageNumberStr)
	if err != nil {
		return
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil {
		return
	}
	if pageNumber-1 < 0 {
		pageNumber = 1
	}
	paginatedQuery = fmt.Sprintf("LIMIT %d OFFSET(%d - 1) * %d", pageSize, pageNumber, pageSize)
	return
}

func (fr *FuncRegistry) limitOffset(pageNumber, pageSize string) (value string) {
	value, err := LimitOffset(pageNumber, pageSize)
	if err != nil {
		value = ""
	}
	return
}

// sqlVal returns a positional placeholder for a single value and stores it in Args
func (fr *FuncRegistry) sqlVal(key string) string {
	v := fr.TemplateData[key]
	fr.Args = append(fr.Args, v)
	fr.next++
	return fmt.Sprintf("$%d", fr.next)
}

// sqlList returns a parenthesized, comma-separated list of placeholders for a slice value
func (fr *FuncRegistry) sqlList(key string) string {
	if s, ok := fr.TemplateData[key].([]string); ok {
		ph := make([]string, len(s))
		for i := range s {
			fr.Args = append(fr.Args, s[i])
			fr.next++
			ph[i] = fmt.Sprintf("$%d", fr.next)
		}
		return fmt.Sprintf("(%s)", strings.Join(ph, ","))
	}
	fr.Args = append(fr.Args, fr.TemplateData[key])
	fr.next++
	return fmt.Sprintf("($%d)", fr.next)
}

// ident validates and safely quotes an identifier (optionally dotted path)
func (fr *FuncRegistry) ident(key string) (string, error) {
	s, _ := fr.TemplateData[key].(string)
	return ident.Quote(s)
}

// workspaceScopeIn emits `<quoted col> IN ($1, $2, …)` for the
// caller's resolved workspace membership, taken from the
// `workspaceIds` template data slot (populated by
// controllers/sql.go::extractContextValues from pctx.WorkspaceIDsKey).
//
// When the list is missing or empty, it emits `FALSE` so
// cross-workspace reads return nothing for users with no workspaces,
// matching the fail-closed policy for the four workspace tables.
//
// Example template usage:
//
//	{{ workspaceScopeIn "t.workspace_id" }}
//
// The column is validated through internal/ident.Quote to reject
// SQL injection — arbitrary expressions are not allowed, only dotted
// identifiers like `t.workspace_id`.
func (fr *FuncRegistry) workspaceScopeIn(col string) string {
	quoted, err := ident.Quote(col)
	if err != nil {
		// Invalid identifier — fail safe rather than emit raw text.
		return "FALSE"
	}
	raw, ok := fr.TemplateData["workspaceIds"].([]string)
	if !ok || len(raw) == 0 {
		return "FALSE"
	}
	ph := make([]string, len(raw))
	for i := range raw {
		fr.Args = append(fr.Args, raw[i])
		fr.next++
		ph[i] = fmt.Sprintf("$%d", fr.next)
	}
	return fmt.Sprintf("%s IN (%s)", quoted, strings.Join(ph, ","))
}
