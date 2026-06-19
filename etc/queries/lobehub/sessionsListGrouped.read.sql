-- sessionsListGrouped
-- Replaces: routers/lambda/session.ts: getGroupedSessions
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — if set, scope to workspace)
--               workspaceScope (optional "all" — cross-workspace mode,
--                               resolved via Keto membership; else personal
--                               scope with workspace_id IS NULL)
--
-- Query params:
--   includePinnedOnly (bool, default false) — filter to pinned sessions only
--   page              (int, default 1)
--   size              (int, default 20)
--
-- Returns: array of sessions with joined group columns and per-session
--          topic counts.
SELECT
    s.id,
    s.slug,
    s.title,
    s.description,
    s.avatar,
    s.background_color,
    s.type,
    s.pinned,
    s.group_id,
    s.client_id,
    s.created_at,
    s.updated_at,
    g.name        AS group_name,
    g.sort        AS group_sort,
    g.client_id   AS group_client_id,
    COALESCE(t.cnt, 0)::int AS topic_count
FROM   sessions s
LEFT JOIN session_groups g
       ON g.id = s.group_id
{{- if isSet "workspaceId" }}
      AND g.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
      AND {{ workspaceScopeIn "g.workspace_id" }}
{{- else }}
      AND g.workspace_id IS NULL
{{- end }}
LEFT JOIN (
    SELECT session_id, COUNT(*) AS cnt
    FROM   topics
    {{- if isSet "workspaceId" }}
    WHERE  workspace_id = {{ sqlVal "workspaceId" }}
    {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
    WHERE  {{ workspaceScopeIn "workspace_id" }}
    {{- else }}
    WHERE  user_id = {{ sqlVal "userId" }} AND workspace_id IS NULL
    {{- end }}
    GROUP  BY session_id
) t ON t.session_id = s.id
{{- if isSet "workspaceId" }}
WHERE  s.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "s.workspace_id" }}
{{- else }}
WHERE  s.user_id = {{ sqlVal "userId" }} AND s.workspace_id IS NULL
{{- end }}
{{- if eq (defaultOrValue "includePinnedOnly" "false") "true" }}
  AND  s.pinned = true
{{- end }}
ORDER  BY g.sort NULLS LAST, s.pinned DESC, s.updated_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
