package controllers

import (
	"net/http"
)

// RequireWorkspaceMembership is a gorilla/mux middleware that enforces the
// caller has an active workspace (X-Tenant-Id header, injected by Oathkeeper
// from the Kratos session metadata_public) and is a member of it.
//
// This is defense-in-depth — Oathkeeper's remote_json authorizer is the
// primary gate. If pREST is reached directly (edge bypass), the X-Tenant-Id
// header will be absent and this middleware rejects the request.
func RequireWorkspaceMembership(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wsID := r.Header.Get("X-Tenant-Id")
		if wsID == "" {
			writeJSONError(w, http.StatusForbidden, "no active workspace")
			return
		}
		u := userFromReq(r)
		if u.ID != "" && !userIsMember(r.Context(), kawaiDB(), wsID, u.ID) {
			writeJSONError(w, http.StatusForbidden, "not a member of this workspace")
			return
		}
		next.ServeHTTP(w, r)
	})
}
