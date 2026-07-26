package router

import (
	"runtime"

	"github.com/prest/prest/v2/config"
	"github.com/prest/prest/v2/controllers"
	"github.com/prest/prest/v2/middlewares"
	"github.com/prest/prest/v2/plugins"

	"github.com/gorilla/mux"
	"github.com/urfave/negroni/v3"
)

// GetRouter reagister all routes
// v2: this is not used anywhere, so we can make it private
func GetRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/databases", controllers.GetDatabases).Methods("GET")
	router.HandleFunc("/schemas", controllers.GetSchemas).Methods("GET")
	router.HandleFunc("/tables", controllers.GetTables).Methods("GET")
	// breaking change
	router.HandleFunc("/_QUERIES/{queriesLocation}/{script}", controllers.ExecuteFromScripts)
	// router.HandleFunc("/_QUERIES/{database}/{queriesLocation}/{script}", controllers.ExecuteFromScripts)
	// if it is windows it should not register the plugin endpoint
	// we use go plugin system that does not support windows
	// https://github.com/golang/go/issues/19282
	if runtime.GOOS != "windows" {
		router.HandleFunc("/_PLUGIN/{file}/{func}", plugins.HandlerPlugin)
	}
	router.HandleFunc("/{database}/{schema}", controllers.GetTablesByDatabaseAndSchema).Methods("GET")
	router.HandleFunc("/show/{database}/{schema}/{table}", controllers.ShowTable).Methods("GET")
	crudRoutes := mux.NewRouter().PathPrefix("/").Subrouter().StrictSlash(true)
	router.HandleFunc("/_health", controllers.WrappedHealthCheck(controllers.DefaultCheckList)).Methods("GET")
	// Account self-service closure (purges Kawai data + Kratos identity).
	router.HandleFunc("/v1/account/delete", controllers.AccountDeleteHandler).Methods("POST")

	// Workspace management (tenants + members + invites). Edge rule
	// prest-tenants-v1 injects X-User-Id + X-User-Email; each handler
	// enforces tenant-level authz.
	router.HandleFunc("/v1/workspaces", controllers.TenantsListHandler).Methods("GET")
	router.HandleFunc("/v1/workspaces", controllers.TenantsCreateHandler).Methods("POST")
	router.HandleFunc("/v1/workspaces/invites/accept", controllers.TenantInviteAcceptHandler).Methods("POST")
	router.HandleFunc("/v1/workspaces/{id}", controllers.TenantGetHandler).Methods("GET")
	router.HandleFunc("/v1/workspaces/{id}", controllers.TenantRenameHandler).Methods("PATCH")
	router.HandleFunc("/v1/workspaces/{id}", controllers.TenantDeleteHandler).Methods("DELETE")
	router.HandleFunc("/v1/workspaces/{id}/members", controllers.TenantMembersHandler).Methods("GET")
	router.HandleFunc("/v1/workspaces/{id}/members/{membershipId}", controllers.MemberUpdateRoleHandler).Methods("PATCH")
	router.HandleFunc("/v1/workspaces/{id}/members/{membershipId}", controllers.MemberRemoveHandler).Methods("DELETE")
	router.HandleFunc("/v1/workspaces/{id}/invites", controllers.TenantInviteCreateHandler).Methods("POST")

	// Scheduled runs — polls scheduled_runs and enqueues agent_run jobs.
	router.HandleFunc("/v1/workspaces/{id}/schedules", controllers.SchedulesListHandler).Methods("GET")
	router.HandleFunc("/v1/workspaces/{id}/schedules", controllers.SchedulesCreateHandler).Methods("POST")
	router.HandleFunc("/v1/workspaces/{id}/schedules/{schedId}", controllers.SchedulesDeleteHandler).Methods("DELETE")
	router.HandleFunc("/v1/workspaces/{id}/schedules/{schedId}/runs", controllers.SchedulesRunsHandler).Methods("GET")
	router.HandleFunc("/v1/workspaces/{id}/schedule-runs", controllers.ScheduledRunsAllExecutionsHandler).Methods("GET")

	// Oathkeeper remote_json target for workspace authz (prest-tenant-* rules).
	// Returns {"allowed": bool}. Registered on the top-level router so it
	// bypasses the per-CRUD user-scope middleware (it's an internal authz call).
	router.HandleFunc("/authz/workspace", controllers.AuthzWorkspaceHandler).Methods("POST")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.SelectFromTables).Methods("GET")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.InsertInTables).Methods("POST")
	crudRoutes.HandleFunc("/batch/{database}/{schema}/{table}", controllers.BatchInsertInTables).Methods("POST")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.DeleteFromTable).Methods("DELETE")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.UpdateTable).Methods("PUT", "PATCH")
	router.PathPrefix("/").Handler(negroni.New(
		middlewares.AccessControl(),
		middlewares.ExposureMiddleware(),
		middlewares.UserFilterMiddleware(),
		middlewares.TenantActiveMiddleware(),
		middlewares.CacheMiddleware(&config.PrestConf.Cache),
		// plugins middleware
		plugins.MiddlewarePlugin(),
		negroni.Wrap(crudRoutes),
	))

	return router
}

// Routes for pREST
func Routes() *negroni.Negroni {
	n := middlewares.GetApp()
	n.UseHandler(GetRouter())
	return n
}
