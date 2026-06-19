package controllers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/prest/prest/v2/config"
	pctx "github.com/prest/prest/v2/context"

	"log/slog"
)

// CheckAuthz checks a workspace permission via Ory Keto.
//
// Query params:
//   - namespace   (default "workspace")
//   - object      (required — e.g. workspace ID)
//   - relation    (required — e.g. "view", "write", "manage")
//   - subject_id  (optional — defaults to authenticated user from X-User-Id header)
//
// Calls Keto's Check API and returns {"allowed":true|false}.
func CheckAuthz(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	namespace := q.Get("namespace")
	if namespace == "" {
		namespace = "workspace"
	}
	object := q.Get("object")
	relation := q.Get("relation")
	subjectID := q.Get("subject_id")
	if subjectID == "" {
		if id, ok := r.Context().Value(pctx.UserIDKey).(string); ok && id != "" {
			subjectID = id
		}
	}

	switch {
	case object == "":
		jsonError(w, "missing required query param: object", http.StatusBadRequest)
		return
	case relation == "":
		jsonError(w, "missing required query param: relation", http.StatusBadRequest)
		return
	case subjectID == "":
		jsonError(w, "missing subject_id (provide as query param or authenticate via X-User-Id header)", http.StatusUnauthorized)
		return
	}

	checkURL := fmt.Sprintf(
		"%s/relation-tuples/check?namespace=%s&object=%s&relation=%s&subject_id=%s",
		config.PrestConf.KetoReadURL,
		url.QueryEscape(namespace),
		url.QueryEscape(object),
		url.QueryEscape(relation),
		url.QueryEscape(subjectID),
	)

	client := &http.Client{}
	resp, err := client.Get(checkURL)
	if err != nil {
		slog.Error("keto check request failed", "err", err)
		jsonError(w, "authz service unavailable", http.StatusServiceUnavailable)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("keto read response body", "err", err)
		jsonError(w, "authz service error", http.StatusServiceUnavailable)
		return
	}

	// Keto returns {"allowed":true} or {"allowed":false}
	var ketoResp struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.Unmarshal(body, &ketoResp); err != nil {
		slog.Error("keto response parse", "err", err, "body", string(body))
		jsonError(w, "authz response parse error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ketoResp)
}
