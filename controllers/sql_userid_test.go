// nolint
package controllers

import (
	"context"
	"net/http/httptest"
	"testing"

	pctx "github.com/prest/prest/v2/context"
)

func TestExtractContextValuesInjectsUserID(t *testing.T) {
	req := httptest.NewRequest("GET", "/_QUERIES/lobehub/sessionsListGrouped", nil)
	req = req.WithContext(context.WithValue(req.Context(), pctx.UserIDKey, "identity-abc"))

	data := map[string]interface{}{}
	extractContextValues(req, data)

	if got, want := data["userId"], "identity-abc"; got != want {
		t.Fatalf("userId = %v, want %v", got, want)
	}
}

func TestExtractContextValuesNoUserID(t *testing.T) {
	req := httptest.NewRequest("GET", "/_health", nil)
	data := map[string]interface{}{}
	extractContextValues(req, data)

	if _, present := data["userId"]; present {
		t.Fatalf("userId unexpectedly set on public request: %v", data["userId"])
	}
}
