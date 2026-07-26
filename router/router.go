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
	router.HandleFunc("/show/{database}/{schema}/{table}", controllers.ShowTable).Methods("GET")
	crudRoutes := mux.NewRouter().PathPrefix("/").Subrouter().StrictSlash(true)
	router.HandleFunc("/_health", controllers.WrappedHealthCheck(controllers.DefaultCheckList)).Methods("GET")
	// Account self-service (not workspace-scoped — no X-Tenant-Id needed).
	router.HandleFunc("/v1/account/delete", controllers.AccountDeleteHandler).Methods("POST")
	router.HandleFunc("/v1/account/active-workspace", controllers.ActiveWorkspaceGetHandler).Methods("GET")
	router.HandleFunc("/v1/account/active-workspace", controllers.ActiveWorkspaceSetHandler).Methods("PATCH")

	// Workspace management — bare routes (no active workspace needed: list,
	// create, accept invite). Registered on the main router (NOT a subrouter)
	// because gorilla/mux PathPrefix subrouters don't match the empty-path
	// case (""). These must come BEFORE the scoped subrouter below.
	router.HandleFunc("/v1/workspaces", controllers.TenantsListHandler).Methods("GET")
	router.HandleFunc("/v1/workspaces", controllers.TenantsCreateHandler).Methods("POST")
	router.HandleFunc("/v1/workspaces/invites/accept", controllers.TenantInviteAcceptHandler).Methods("POST")

	// Workspace-scoped routes — RequireWorkspaceMembership enforces X-Tenant-Id
	// (injected by Oathkeeper from Kratos metadata_public.active_workspace_id).
	wsScoped := router.PathPrefix("/v1/workspaces").Subrouter()
	wsScoped.Use(controllers.RequireWorkspaceMembership)
	wsScoped.HandleFunc("/current", controllers.TenantGetHandler).Methods("GET")
	wsScoped.HandleFunc("/current", controllers.TenantRenameHandler).Methods("PATCH")
	wsScoped.HandleFunc("/current", controllers.TenantDeleteHandler).Methods("DELETE")
	wsScoped.HandleFunc("/members", controllers.TenantMembersHandler).Methods("GET")
	wsScoped.HandleFunc("/members/{membershipId}", controllers.MemberUpdateRoleHandler).Methods("PATCH")
	wsScoped.HandleFunc("/members/{membershipId}", controllers.MemberRemoveHandler).Methods("DELETE")
	wsScoped.HandleFunc("/invites", controllers.TenantInviteCreateHandler).Methods("POST")
	wsScoped.HandleFunc("/schedules", controllers.SchedulesListHandler).Methods("GET")
	wsScoped.HandleFunc("/schedules", controllers.SchedulesCreateHandler).Methods("POST")
	wsScoped.HandleFunc("/schedules/{schedId}", controllers.SchedulesDeleteHandler).Methods("DELETE")
	wsScoped.HandleFunc("/schedules/{schedId}/runs", controllers.SchedulesRunsHandler).Methods("GET")
	wsScoped.HandleFunc("/schedule-runs", controllers.ScheduledRunsAllExecutionsHandler).Methods("GET")

	// Oathkeeper remote_json target for workspace authz (prest-tenant-* rules).
	// Returns {"allowed": bool}. Registered on the top-level router so it
	// bypasses the per-CRUD user-scope middleware (it's an internal authz call).
	router.HandleFunc("/authz/workspace", controllers.AuthzWorkspaceHandler).Methods("POST")
	router.HandleFunc("/{database}/{schema}", controllers.GetTablesByDatabaseAndSchema).Methods("GET")
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
