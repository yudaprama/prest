-- agentSharesByUser
-- Replaces: (no BFF router yet — schema-only in packages/database/src/schemas/agentShare.ts)
--           Provides the read path for listing agent shares owned by the caller.
--           `agent_shares` has no `user_id` column; scoping goes through
--           `agents.user_id` via a JOIN.
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — if set, scope to workspace)
--               workspaceScope (optional "all" — cross-workspace mode;
--                               resolved via Keto membership; else personal
--                               scope with workspace_id IS NULL)
--
-- Query params:
--   visibility (text, optional) — 'private' | 'link'
--   page       (int, default 1)
--   size       (int, default 20)
--
-- Returns: array of agent_shares joined with their parent agent's id/title/slug.
SELECT
    s.id,
    s.agent_id,
    s.visibility,
    s.share_config,
    s.user_view_count,
    s.created_at,
    s.updated_at,
    a.title  AS agent_title,
    a.slug   AS agent_slug
FROM   agent_shares s
JOIN   agents a ON a.id = s.agent_id
{{- if isSet "workspaceId" }}
WHERE  a.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "a.workspace_id" }}
{{- else }}
WHERE  a.user_id = {{ sqlVal "userId" }} AND a.workspace_id IS NULL
{{- end }}
{{- if isSet "visibility" }}
  AND  s.visibility = {{ sqlVal "visibility" }}
{{- end }}
ORDER  BY s.updated_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
