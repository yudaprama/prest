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
	// Workspace authz gate for the Oathkeeper edge (remote_json). Registered on
	// the top-level router so it bypasses the per-CRUD user-scope middleware —
	// it's an internal authz call, not a user-scoped data request.
	router.Handle("/authz/workspace", controllers.AuthzWorkspaceHandler()).Methods("POST")
	// Workspace management surface (CRUD + members + Kratos signup bootstrap).
	// Ported from egent-lobehub; registered on the top-level router so these
	// run their own Keto-based authz rather than the per-CRUD user-scope chain.
	// /internal/* is loopback-only (not routed by the public edge).
	router.HandleFunc("/v1/workspaces", controllers.WorkspacesHandler).Methods("POST", "DELETE")
	router.HandleFunc("/v1/workspaces/members", controllers.WorkspaceMembersHandler).Methods("POST")
	router.HandleFunc("/v1/workspaces/members/remove", controllers.WorkspaceRemoveMemberHandler).Methods("POST")
	router.HandleFunc("/v1/workspaces/leave", controllers.WorkspaceLeaveHandler).Methods("POST")
	// Account self-service closure (purges Kawai + workspaces + Keto + Kratos
	// identity). Same cookie-authed edge rule as /v1/workspaces.* (prest-workspaces-v1).
	router.HandleFunc("/v1/account/delete", controllers.AccountDeleteHandler).Methods("POST")
	router.HandleFunc("/internal/workspaces/bootstrap", controllers.InternalWorkspaceBootstrapHandler).Methods("POST")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.SelectFromTables).Methods("GET")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.InsertInTables).Methods("POST")
	crudRoutes.HandleFunc("/batch/{database}/{schema}/{table}", controllers.BatchInsertInTables).Methods("POST")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.DeleteFromTable).Methods("DELETE")
	crudRoutes.HandleFunc("/{database}/{schema}/{table}", controllers.UpdateTable).Methods("PUT", "PATCH")
	router.PathPrefix("/").Handler(negroni.New(
		middlewares.AccessControl(),
		middlewares.ExposureMiddleware(),
		middlewares.UserFilterMiddleware(),
		middlewares.WorkspaceActiveMiddleware(),
		middlewares.WorkspaceMembershipResolver(),
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
