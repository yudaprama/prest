// Package controllers — scheduled runs CRUD endpoints. Manages
// cron/scheduled-run triggers via kawai-owned pREST handlers.
//
// Endpoints (workspace-scoped):
//
//	GET    /v1/workspaces/{id}/schedules          — list schedules for user in workspace
//	POST   /v1/workspaces/{id}/schedules          — create a cron or one-shot schedule
//	DELETE /v1/workspaces/{id}/schedules/{schedId} — delete a schedule
//	GET    /v1/workspaces/{id}/schedules/{schedId}/runs — execution history for a schedule
package controllers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// ── Request / response types ────────────────────────────────────────────────

type scheduleRow struct {
	ID          string  `json:"id"`
	UserID      string  `json:"userId"`
	WorkspaceID string  `json:"workspaceId"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"` // "cron" | "once"
	Expression  *string `json:"expression,omitempty"`
	TriggerAt   *string `json:"triggerAt,omitempty"`
	AgentID     *string `json:"agentId,omitempty"`
	Goal        string  `json:"goal"`
	Model       *string `json:"model,omitempty"`
	Enabled     bool    `json:"enabled"`
	NextRunAt   *string `json:"nextRunAt,omitempty"`
	LastRunAt   *string `json:"lastRunAt,omitempty"`
	TotalRuns   int     `json:"totalRuns"`
	CreatedAt   string  `json:"createdAt"`
}

type executionRow struct {
	ID         string  `json:"id"`
	ScheduleID string  `json:"scheduleId"`
	Status     string  `json:"status"`
	StartedAt  *string `json:"startedAt,omitempty"`
	FinishedAt *string `json:"finishedAt,omitempty"`
	DurationMs *int64  `json:"durationMs,omitempty"`
	Output     *string `json:"output,omitempty"`
	Error      *string `json:"error,omitempty"`
	CreatedAt  string  `json:"createdAt"`
}

type createScheduleReq struct {
	Name       string  `json:"name"`
	Goal       string  `json:"goal"`
	Kind       string  `json:"kind"` // "cron" | "once"
	Expression *string `json:"expression,omitempty"`
	TriggerAt  *string `json:"triggerAt,omitempty"`
	AgentID    *string `json:"agentId,omitempty"`
	Model      *string `json:"model,omitempty"`
}

// ── List schedules ──────────────────────────────────────────────────────────

// SchedulesListHandler returns all schedules owned by the caller in a workspace.
// GET /v1/workspaces/{id}/schedules
func SchedulesListHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing user")
		return
	}
	wsID := mux.Vars(r)["id"]
	if wsID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace id required")
		return
	}
	db := kawaiDB()

	rows, err := db.Query(r.Context(),
		`SELECT id, user_id, workspace_id, name, kind, expression, trigger_at, agent_id,
		        goal, model, enabled, next_run_at, last_run_at, total_runs, created_at
		 FROM scheduled_runs
		 WHERE workspace_id = $1 AND user_id = $2
		 ORDER BY created_at DESC`, wsID, u.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query: "+err.Error())
		return
	}
	defer rows.Close()

	out := make([]scheduleRow, 0)
	for rows.Next() {
		var s scheduleRow
		var expr, trig, agent, model, next, last *string
		if err := rows.Scan(&s.ID, &s.UserID, &s.WorkspaceID, &s.Name, &s.Kind,
			&expr, &trig, &agent, &s.Goal, &model, &s.Enabled,
			&next, &last, &s.TotalRuns, &s.CreatedAt); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "scan: "+err.Error())
			return
		}
		s.Expression = expr
		s.TriggerAt = trig
		s.AgentID = agent
		s.Model = model
		s.NextRunAt = next
		s.LastRunAt = last
		out = append(out, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out})
}

// ── Create schedule ─────────────────────────────────────────────────────────

// SchedulesCreateHandler inserts a new schedule. The scheduler worker
// (egent-jobs) polls enabled rows and fires River agent_run jobs when due.
// POST /v1/workspaces/{id}/schedules
func SchedulesCreateHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing user")
		return
	}
	wsID := mux.Vars(r)["id"]
	if wsID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace id required")
		return
	}
	var in createScheduleReq
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Goal = strings.TrimSpace(in.Goal)
	if in.Goal == "" {
		writeJSONError(w, http.StatusBadRequest, "goal required")
		return
	}
	if in.Name == "" {
		in.Name = in.Goal
		if len(in.Name) > 60 {
			in.Name = in.Name[:60]
		}
	}
	if in.Kind != "cron" && in.Kind != "once" {
		writeJSONError(w, http.StatusBadRequest, "kind must be 'cron' or 'once'")
		return
	}
	if in.Kind == "cron" && (in.Expression == nil || strings.TrimSpace(*in.Expression) == "") {
		writeJSONError(w, http.StatusBadRequest, "expression required for cron")
		return
	}
	if in.Kind == "once" && (in.TriggerAt == nil || strings.TrimSpace(*in.TriggerAt) == "") {
		writeJSONError(w, http.StatusBadRequest, "triggerAt required for once")
		return
	}

	db := kawaiDB()

	id, uuidErr := newUUID()
	if uuidErr != nil {
		writeJSONError(w, http.StatusInternalServerError, "uuid: "+uuidErr.Error())
		return
	}
	now := time.Now().UTC()

	// Compute next_run_at for the scheduler.
	var nextRunAt *time.Time
	if in.Kind == "cron" && in.Expression != nil {
		nextRunAt = &now // scheduler will re-compute
	}
	if in.Kind == "once" && in.TriggerAt != nil {
		if t, err := time.Parse(time.RFC3339, *in.TriggerAt); err == nil {
			nextRunAt = &t
		}
	}

	_, err := db.Exec(r.Context(),
		`INSERT INTO scheduled_runs
		     (id, user_id, workspace_id, name, kind, expression, trigger_at, agent_id, goal, model, enabled, next_run_at, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,true,$11,$12,$12)`,
		id, u.ID, wsID, in.Name, in.Kind,
		in.Expression, in.TriggerAt, in.AgentID, in.Goal, in.Model,
		nextRunAt, now)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "insert: "+err.Error())
		return
	}

	var s scheduleRow
	s.ID = id
	s.UserID = u.ID
	s.WorkspaceID = wsID
	s.Name = in.Name
	s.Kind = in.Kind
	s.Expression = in.Expression
	s.TriggerAt = in.TriggerAt
	s.AgentID = in.AgentID
	s.Goal = in.Goal
	s.Model = in.Model
	s.Enabled = true
	if nextRunAt != nil {
		t := nextRunAt.Format(time.RFC3339)
		s.NextRunAt = &t
	}
	s.TotalRuns = 0
	s.CreatedAt = now.Format(time.RFC3339)

	writeJSON(w, http.StatusCreated, s)
}

// ── Delete schedule ─────────────────────────────────────────────────────────

// SchedulesDeleteHandler removes a schedule (and cascades to execution history).
// DELETE /v1/workspaces/{id}/schedules/{schedId}
func SchedulesDeleteHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing user")
		return
	}
	wsID := mux.Vars(r)["id"]
	schedID := mux.Vars(r)["schedId"]
	if wsID == "" || schedID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace id and schedule id required")
		return
	}
	db := kawaiDB()

	// Ownership check: schedule must belong to caller in this workspace.
	var ownerID string
	if err := db.QueryRow(r.Context(),
		`SELECT user_id FROM scheduled_runs WHERE id = $1 AND workspace_id = $2`,
		schedID, wsID).Scan(&ownerID); err != nil {
		writeJSONError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if ownerID != u.ID {
		writeJSONError(w, http.StatusForbidden, "not your schedule")
		return
	}

	res, err := db.Exec(r.Context(),
		`DELETE FROM scheduled_runs WHERE id = $1`, schedID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "delete: "+err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "schedule not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": schedID, "deleted": true})
}

// ── List executions for a schedule ──────────────────────────────────────────

// SchedulesRunsHandler returns execution history for a specific schedule.
// GET /v1/workspaces/{id}/schedules/{schedId}/runs
func SchedulesRunsHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing user")
		return
	}
	wsID := mux.Vars(r)["id"]
	schedID := mux.Vars(r)["schedId"]
	if wsID == "" || schedID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace id and schedule id required")
		return
	}
	db := kawaiDB()

	// Verify caller owns this schedule in this workspace.
	var ownerID string
	if err := db.QueryRow(r.Context(),
		`SELECT user_id FROM scheduled_runs WHERE id = $1 AND workspace_id = $2`,
		schedID, wsID).Scan(&ownerID); err != nil {
		writeJSONError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if ownerID != u.ID {
		writeJSONError(w, http.StatusForbidden, "not your schedule")
		return
	}

	rows, err := db.Query(r.Context(),
		`SELECT id, schedule_id, status, started_at, finished_at, duration_ms, output, error, created_at
		 FROM scheduled_run_executions
		 WHERE schedule_id = $1
		 ORDER BY created_at DESC
		 LIMIT 100`, schedID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query: "+err.Error())
		return
	}
	defer rows.Close()

	out := make([]executionRow, 0)
	for rows.Next() {
		var e executionRow
		var started, finished, output, errMsg *string
		var durMs *int64
		if err := rows.Scan(&e.ID, &e.ScheduleID, &e.Status,
			&started, &finished, &durMs, &output, &errMsg, &e.CreatedAt); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "scan: "+err.Error())
			return
		}
		e.StartedAt = started
		e.FinishedAt = finished
		e.DurationMs = durMs
		e.Output = output
		e.Error = errMsg
		out = append(out, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out})
}

// ── List ALL executions across user's schedules (for /tasks runs view) ──────

// ScheduledRunsAllExecutionsHandler returns recent executions across all of the
// caller's schedules in a workspace. Used by the SPA "Tasks" view to show a
// unified run history.
// GET /v1/workspaces/{id}/schedule-runs
func ScheduledRunsAllExecutionsHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing user")
		return
	}
	wsID := mux.Vars(r)["id"]
	if wsID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace id required")
		return
	}
	db := kawaiDB()

	rows, err := db.Query(r.Context(),
		`SELECT e.id, e.schedule_id, e.status, e.started_at, e.finished_at,
		        e.duration_ms, e.output, e.error, e.created_at,
		        s.name, s.agent_id, s.goal
		 FROM scheduled_run_executions e
		 JOIN scheduled_runs s ON s.id = e.schedule_id
		 WHERE s.workspace_id = $1 AND s.user_id = $2
		 ORDER BY e.created_at DESC
		 LIMIT 200`, wsID, u.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query: "+err.Error())
		return
	}
	defer rows.Close()

	type runWithMeta struct {
		executionRow
		Name    *string `json:"name,omitempty"`
		AgentID *string `json:"agentId,omitempty"`
		Goal    *string `json:"goal,omitempty"`
	}
	out := make([]runWithMeta, 0)
	for rows.Next() {
		var r runWithMeta
		var started, finished, output, errMsg, name, agent, goal *string
		var durMs *int64
		if err := rows.Scan(&r.ID, &r.ScheduleID, &r.Status,
			&started, &finished, &durMs, &output, &errMsg, &r.CreatedAt,
			&name, &agent, &goal); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "scan: "+err.Error())
			return
		}
		r.StartedAt = started
		r.FinishedAt = finished
		r.DurationMs = durMs
		r.Output = output
		r.Error = errMsg
		r.Name = name
		r.AgentID = agent
		r.Goal = goal
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out})
}