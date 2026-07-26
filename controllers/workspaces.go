package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/gorilla/mux"
)

// Tenant / membership / invite management, hosted on pREST.
//
// Auth contract: every request arrives with X-User-Id (Kratos identity.id,
// authoritative — injected by Oathkeeper) and X-User-Email (likewise). The
// edge rule (prest-tenants-v1) gates access; these handlers enforce
// tenant-level authorization by inspecting tenant_members directly.
//
// Response shape is intentionally simple (flat JSON, no metadata envelope).
// The web SPA (web/src/lib/workspaces.ts) maps these into its UI types.

const (
	roleOwner  = "owner"
	roleMember = "member"
	roleViewer = "viewer"
)

func validRole(r string) bool {
	return r == roleOwner || r == roleMember || r == roleViewer
}

// userCtx bundles the edge-injected identity for each request.
type userCtx struct {
	ID    string
	Email string
}

func userFromReq(r *http.Request) userCtx {
	return userCtx{
		ID:    r.Header.Get("X-User-Id"),
		Email: r.Header.Get("X-User-Email"),
	}
}

// Tenant row (response shape).
type tenantRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	CreatedBy string `json:"createdBy,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

// Member row (response shape).
type memberRow struct {
	MembershipID string `json:"membershipId"` // synthetic — composed from tenant_id+user_id
	TenantID     string `json:"tenantId"`
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	CreatedAt    string `json:"createdAt,omitempty"`
}

// Invite row (response shape).
type inviteRow struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	InvitedBy string `json:"invitedBy,omitempty"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// ───────────────────────── handlers ─────────────────────────

// TenantsListHandler: GET /v1/workspaces — list the caller's memberships.
func TenantsListHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	db := kawaiDB()

	rows, err := db.Query(r.Context(),
		`SELECT t.id, t.name, t.slug, m.role, t.created_at, t.updated_at
		 FROM tenants t
		 JOIN tenant_members m ON m.tenant_id = t.id
		 WHERE m.user_id = $1
		 ORDER BY t.created_at DESC`, u.ID)
	if err != nil {
		slog.Error("tenants list: query", "err", err)
		writeJSONError(w, http.StatusBadGateway, "query failed")
		return
	}
	defer rows.Close()

	type membershipItem struct {
		tenantRow
		Role string `json:"role"`
	}
	out := []membershipItem{}
	for rows.Next() {
		var t tenantRow
		var role string
		var createdAt, updatedAt *string
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &role, &createdAt, &updatedAt); err != nil {
			slog.Error("tenants list: scan", "err", err)
			writeJSONError(w, http.StatusBadGateway, "scan failed")
			return
		}
		if createdAt != nil {
			t.CreatedAt = *createdAt
		}
		if updatedAt != nil {
			t.UpdatedAt = *updatedAt
		}
		out = append(out, membershipItem{tenantRow: t, Role: role})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out})
}

// TenantsCreateHandler: POST /v1/workspaces {name, slug?} — create + auto-owner.
func TenantsCreateHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	if body.Slug == "" {
		body.Slug = slugify(body.Name)
	}
	if !validRole(roleOwner) {
		writeJSONError(w, http.StatusInternalServerError, "role table misconfigured")
		return
	}

	db := kawaiDB()

	id, err := newUUID()
	if err != nil {
		slog.Error("tenants create: uuid", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "id generation failed")
		return
	}

	ctx := r.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		slog.Error("tenants create: begin tx", "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not create workspace")
		return
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenants (id, slug, name, created_by, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $5)`,
		id, body.Slug, body.Name, u.ID, now); err != nil {
		slog.Error("tenants create: insert tenant", "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not create workspace")
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenant_members (tenant_id, user_id, email, role, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, u.ID, strings.ToLower(u.Email), roleOwner, now); err != nil {
		slog.Error("tenants create: insert member", "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not create workspace")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("tenants create: commit", "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not create workspace")
		return
	}

	writeJSON(w, http.StatusOK, tenantRow{ID: id, Name: body.Name, Slug: body.Slug, CreatedBy: u.ID})
}

// TenantGetHandler: GET /v1/workspaces/{id} — fetch one workspace (member only).
func TenantGetHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.Header.Get("X-Tenant-Id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "id required")
		return
	}
	db := kawaiDB()

	if !userIsMember(r.Context(), db, id, u.ID) {
		writeJSONError(w, http.StatusForbidden, "not a member")
		return
	}
	var t tenantRow
	var createdAt, updatedAt *string
	err := db.QueryRow(r.Context(),
		`SELECT id, name, slug, created_at, updated_at FROM tenants WHERE id = $1`, id).
		Scan(&t.ID, &t.Name, &t.Slug, &createdAt, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		slog.Error("tenant get: query", "err", err)
		writeJSONError(w, http.StatusBadGateway, "query failed")
		return
	}
	if createdAt != nil {
		t.CreatedAt = *createdAt
	}
	if updatedAt != nil {
		t.UpdatedAt = *updatedAt
	}
	writeJSON(w, http.StatusOK, t)
}

// TenantRenameHandler: PATCH /v1/workspaces/{id} {name} — owner/member only.
func TenantRenameHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.Header.Get("X-Tenant-Id")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, "name required")
		return
	}
	db := kawaiDB()

	if !userCanWrite(r.Context(), db, id, u.ID) {
		writeJSONError(w, http.StatusForbidden, "insufficient permission")
		return
	}
	res, err := db.Exec(r.Context(),
		`UPDATE tenants SET name = $1, updated_at = now() WHERE id = $2`, body.Name, id)
	if err != nil {
		slog.Error("tenant rename: exec", "err", err)
		writeJSONError(w, http.StatusBadGateway, "update failed")
		return
	}
	if res.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "name": body.Name})
}

// TenantDeleteHandler: DELETE /v1/workspaces/{id} — owner only. FK cascade
// clears tenant_members and tenant_invites; application tables (sessions etc.)
// have no FK so their tenant_id columns become dangling — that's expected
// (the SPA hides them by membership check).
func TenantDeleteHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.Header.Get("X-Tenant-Id")
	db := kawaiDB()

	if !userIsOwner(r.Context(), db, id, u.ID) {
		writeJSONError(w, http.StatusForbidden, "only an owner can delete a workspace")
		return
	}
	if _, err := db.Exec(r.Context(), `DELETE FROM tenants WHERE id = $1`, id); err != nil {
		slog.Error("tenant delete: exec", "err", err)
		writeJSONError(w, http.StatusBadGateway, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

// TenantMembersHandler: GET /v1/workspaces/{id}/members — list members.
func TenantMembersHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.Header.Get("X-Tenant-Id")
	db := kawaiDB()

	if !userIsMember(r.Context(), db, id, u.ID) {
		writeJSONError(w, http.StatusForbidden, "not a member")
		return
	}
	rows, err := db.Query(r.Context(),
		`SELECT tenant_id, user_id, email, role, created_at
		 FROM tenant_members WHERE tenant_id = $1 ORDER BY created_at ASC`, id)
	if err != nil {
		slog.Error("members list: query", "err", err)
		writeJSONError(w, http.StatusBadGateway, "query failed")
		return
	}
	defer rows.Close()

	out := []memberRow{}
	for rows.Next() {
		var m memberRow
		var createdAt *string
		if err := rows.Scan(&m.TenantID, &m.UserID, &m.Email, &m.Role, &createdAt); err != nil {
			slog.Error("members list: scan", "err", err)
			writeJSONError(w, http.StatusBadGateway, "scan failed")
			return
		}
		m.MembershipID = m.TenantID + ":" + m.UserID
		if createdAt != nil {
			m.CreatedAt = *createdAt
		}
		out = append(out, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"rows": out})
}

// MemberUpdateRoleHandler: PATCH /v1/workspaces/{id}/members/{membershipId} {role}
// — owner only. membershipId is "<tenantId>:<userId>".
func MemberUpdateRoleHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	vars := mux.Vars(r)
	id := vars["id"]
	membershipID := vars["membershipId"]
	targetUserID := membershipUserID(membershipID, id)
	if targetUserID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid membershipId")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !validRole(body.Role) {
		writeJSONError(w, http.StatusBadRequest, "role must be owner|member|viewer")
		return
	}

	db := kawaiDB()

	if !userIsOwner(r.Context(), db, id, u.ID) {
		writeJSONError(w, http.StatusForbidden, "only an owner can manage members")
		return
	}
	// Guard: never let the last owner demote themselves (would strand the ws).
	if targetUserID == u.ID && body.Role != roleOwner {
		ownerCount, _ := countOwners(r.Context(), db, id)
		if ownerCount <= 1 {
			writeJSONError(w, http.StatusBadRequest, "cannot demote the last owner")
			return
		}
	}

	res, err := db.Exec(r.Context(),
		`UPDATE tenant_members SET role = $1 WHERE tenant_id = $2 AND user_id = $3`,
		body.Role, id, targetUserID)
	if err != nil {
		slog.Error("member update role: exec", "err", err)
		writeJSONError(w, http.StatusBadGateway, "update failed")
		return
	}
	if res.RowsAffected() == 0 {
		writeJSONError(w, http.StatusNotFound, "member not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"tenantId": id, "userId": targetUserID, "role": body.Role})
}

// MemberRemoveHandler: DELETE /v1/workspaces/{id}/members/{membershipId}
// — owner can remove anyone; non-owner can only remove themselves (leave).
// A sole owner cannot leave (must delete the workspace).
func MemberRemoveHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	vars := mux.Vars(r)
	id := vars["id"]
	membershipID := vars["membershipId"]
	targetUserID := membershipUserID(membershipID, id)
	if targetUserID == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid membershipId")
		return
	}

	db := kawaiDB()

	isOwner := userIsOwner(r.Context(), db, id, u.ID)
	if targetUserID != u.ID && !isOwner {
		writeJSONError(w, http.StatusForbidden, "only an owner can remove others")
		return
	}
	// Self-removal guard: sole owner cannot leave.
	if targetUserID == u.ID {
		role, _ := userRole(r.Context(), db, id, u.ID)
		if role == roleOwner {
			ownerCount, _ := countOwners(r.Context(), db, id)
			if ownerCount <= 1 {
				writeJSONError(w, http.StatusBadRequest, "sole owner cannot leave; delete the workspace instead")
				return
			}
		}
	}

	if _, err := db.Exec(r.Context(),
		`DELETE FROM tenant_members WHERE tenant_id = $1 AND user_id = $2`,
		id, targetUserID); err != nil {
		slog.Error("member remove: exec", "err", err)
		writeJSONError(w, http.StatusBadGateway, "remove failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenantId": id, "userId": targetUserID, "removed": true})
}

// TenantInviteCreateHandler: POST /v1/workspaces/{id}/invites {email, role}
// — owner only. Generates a token; the SPA sends the invite link via email
// out-of-band (no email transport in pREST).
func TenantInviteCreateHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.Header.Get("X-Tenant-Id")
	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		strings.TrimSpace(body.Email) == "" || !validRole(body.Role) {
		writeJSONError(w, http.StatusBadRequest, "email and role (owner|member|viewer) required")
		return
	}
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))

	db := kawaiDB()

	if !userIsOwner(r.Context(), db, id, u.ID) {
		writeJSONError(w, http.StatusForbidden, "only an owner can invite")
		return
	}

	invID, err := newUUID()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "id generation failed")
		return
	}
	token, err := newToken(32)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "token generation failed")
		return
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)

	if _, err := db.Exec(r.Context(),
		`INSERT INTO tenant_invites (id, tenant_id, email, role, token, invited_by, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		invID, id, body.Email, body.Role, token, u.ID, expires); err != nil {
		slog.Error("invite create: insert", "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not create invite")
		return
	}
	writeJSON(w, http.StatusOK, inviteRow{
		ID: invID, TenantID: id, Email: body.Email, Role: body.Role,
		InvitedBy: u.ID, ExpiresAt: expires.Format(time.RFC3339),
	})
}

// TenantInviteAcceptHandler: POST /v1/workspaces/invites/accept {token}
// — any authenticated user. Adds the caller to the tenant as the invited role
// and marks the invite accepted.
func TenantInviteAcceptHandler(w http.ResponseWriter, r *http.Request) {
	u := userFromReq(r)
	if u.ID == "" {
		writeJSONError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Token) == "" {
		writeJSONError(w, http.StatusBadRequest, "token required")
		return
	}

	db := kawaiDB()

	ctx := r.Context()
	tx, err := db.Begin(ctx)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "tx failed")
		return
	}
	defer tx.Rollback(ctx)

	var inv inviteRow
	var expires *string
	err = tx.QueryRow(ctx,
		`SELECT id, tenant_id, email, role, expires_at
		 FROM tenant_invites
		 WHERE token = $1 AND accepted_at IS NULL AND expires_at > now()`,
		body.Token).Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSONError(w, http.StatusNotFound, "invite not found or expired")
		return
	}
	if err != nil {
		slog.Error("invite accept: select", "err", err)
		writeJSONError(w, http.StatusBadGateway, "query failed")
		return
	}

	// The invite was tied to an email. The accepter must match.
	if strings.ToLower(u.Email) != strings.ToLower(inv.Email) {
		writeJSONError(w, http.StatusForbidden, "invite email mismatch")
		return
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx,
		`INSERT INTO tenant_members (tenant_id, user_id, email, role, created_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		inv.TenantID, u.ID, strings.ToLower(u.Email), inv.Role, now); err != nil {
		slog.Error("invite accept: insert member", "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not accept invite")
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE tenant_invites SET accepted_at = now(), accepted_by = $2 WHERE id = $1`,
		inv.ID, u.ID); err != nil {
		slog.Error("invite accept: mark accepted", "err", err)
		writeJSONError(w, http.StatusBadGateway, "could not accept invite")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSONError(w, http.StatusBadGateway, "commit failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"tenantId": inv.TenantID, "role": inv.Role})
}

// ───────────────────────── helpers ─────────────────────────

func userIsMember(ctx context.Context, db *pgxpool.Pool, tenantID, userID string) bool {
	if tenantID == "" || userID == "" {
		return false
	}
	var one int
	err := db.QueryRow(ctx,
		`SELECT 1 FROM tenant_members WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID).Scan(&one)
	return err == nil
}

func userIsOwner(ctx context.Context, db *pgxpool.Pool, tenantID, userID string) bool {
	role, _ := userRole(ctx, db, tenantID, userID)
	return role == roleOwner
}

func userCanWrite(ctx context.Context, db *pgxpool.Pool, tenantID, userID string) bool {
	role, _ := userRole(ctx, db, tenantID, userID)
	return role == roleOwner || role == roleMember
}

func userRole(ctx context.Context, db *pgxpool.Pool, tenantID, userID string) (string, error) {
	var role string
	err := db.QueryRow(ctx,
		`SELECT role FROM tenant_members WHERE tenant_id = $1 AND user_id = $2`,
		tenantID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func countOwners(ctx context.Context, db *pgxpool.Pool, tenantID string) (int, error) {
	var n int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM tenant_members WHERE tenant_id = $1 AND role = 'owner'`,
		tenantID).Scan(&n)
	return n, err
}

// membershipUserID extracts the user_id portion of the "<tenantId>:<userId>"
// membershipID returned by TenantMembersHandler. Returns "" if malformed or
// the tenant prefix doesn't match.
func membershipUserID(membershipID, tenantID string) string {
	parts := strings.SplitN(membershipID, ":", 2)
	if len(parts) != 2 || parts[0] != tenantID {
		return ""
	}
	return parts[1]
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	// RFC 4122 v4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32], nil
}

func newToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// slugify produces a URL-safe slug from a free-form name (lowercase, hyphens,
// collapsed non-alphanumerics). Matches the SPA's existing client-side slugify
// (web/src/lib/workspaces.ts:115) so client and server agree.
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 32 {
		out = out[:32]
	}
	if out == "" {
		out = "workspace"
	}
	return out
}