package controllers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"
)

// ExecuteScriptQuery is a function to execute and return result of script query
func ExecuteScriptQuery(rq *http.Request, queriesPath string, script string) ([]byte, error) {
	config.PrestConf.Adapter.SetDatabase(config.PrestConf.PGDatabase)
	sqlPath, err := config.PrestConf.Adapter.GetScript(rq.Method, queriesPath, script)
	if err != nil {
		err = fmt.Errorf("could not get script %s/%s, %v", queriesPath, script, err)
		return nil, err
	}

	templateData := make(map[string]interface{})
	extractHeaders(rq, templateData)
	extractQueryParameters(rq, templateData)
	extractContextValues(rq, templateData)

	sql, values, err := config.PrestConf.Adapter.ParseScript(sqlPath, templateData)
	if err != nil {
		err = fmt.Errorf("could not parse script %s/%s, %v", queriesPath, script, err)
		return nil, err
	}

	sc := config.PrestConf.Adapter.ExecuteScriptsCtx(rq.Context(), rq.Method, sql, values)
	if sc.Err() != nil {
		err = fmt.Errorf("could not execute sql, check your prest logs")
		return nil, err
	}

	return sc.Bytes(), nil
}

// ExecuteFromScripts is a controller to peform SQL in scripts created by users
func ExecuteFromScripts(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	queriesPath := vars["queriesLocation"]
	script := vars["script"]
	database := vars["database"]

	if database == "" {
		database = config.PrestConf.Adapter.GetDatabase()
	}

	// set db name on ctx
	ctx := context.WithValue(r.Context(), pctx.DBNameKey, database)

	timeout, _ := ctx.Value(pctx.HTTPTimeoutKey).(int)
	ctx, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))
	defer cancel()

	result, err := ExecuteScriptQuery(r.WithContext(ctx), queriesPath, script)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if r.Method == "GET" {
		// Cache arrow if enabled
		config.PrestConf.Cache.BuntSet(r.URL.String(), string(result))
	}
	//nolint
	w.Write(result)
}

// extractContextValues copies values from the request context into the
// template data. This allows SQL templates to reference, for example,
// `{{ sqlVal "userId" }}` to obtain the authenticated Kratos identity
// ID that the auth middleware stored under pctx.UserIDKey, or
// `{{ sqlVal "workspaceId" }}` for the workspace resolved by
// WorkspaceAuthzGate.
func extractContextValues(rq *http.Request, templateData map[string]interface{}) {
	if id, ok := rq.Context().Value(pctx.UserIDKey).(string); ok && id != "" {
		templateData["userId"] = id
	}
	if id, ok := rq.Context().Value(pctx.WorkspaceIDKey).(string); ok && id != "" {
		templateData["workspaceId"] = id
	}
	if ids, ok := rq.Context().Value(pctx.WorkspaceIDsKey).([]string); ok && len(ids) > 0 {
		templateData["workspaceIds"] = ids
	}
}

// extractHeaders gets from the given request the headers and populate the provided templateData accordingly.
func extractHeaders(rq *http.Request, templateData map[string]interface{}) {
	headers := map[string]interface{}{}

	for key, value := range rq.Header {
		if len(value) == 1 {
			headers[key] = value[0]
			continue
		}
		headers[key] = value
	}

	templateData["header"] = headers
}

// extractQueryParameters gets from the given request the query parameters and populate the provided templateData
// accordingly.
func extractQueryParameters(rq *http.Request, templateData map[string]interface{}) {
	for key, value := range rq.URL.Query() {
		if len(value) == 1 {
			templateData[key] = value[0]
			continue
		}
		templateData[key] = value
	}
}
