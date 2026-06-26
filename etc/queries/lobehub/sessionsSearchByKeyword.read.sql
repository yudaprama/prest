-- sessionsSearchByKeyword
-- Replaces: routers/lambda/session.ts: searchSessions
--           (SessionModel.queryByKeyword → findSessionsByKeywords)
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — scope to one workspace)
--               workspaceScope (optional "all" — cross-workspace via Keto)
--
-- Query params:
--   keyword  (string, required) — matched case-insensitively against the
--            linked agent's title / description.
--
-- Returns: agent-type sessions whose associated agent's title or description
--          matches the keyword, joined with that agent's display fields.
--
-- Notes:
--   * The TS model searched with the ParadeDB BM25 `@@@` operator, which is
--     disabled on this fork's Supabase (native Postgres FTS is used instead).
--     That path threw and the catch returned []. This template uses ILIKE —
--     the same approach as `home.searchAgents` — so search actually works.
--   * Restricted to `type = 'agent'`: the mobile search UI (the only consumer)
--     filters group sessions out, and the agent branch is the only shape the
--     frontend adapter needs (no group `members` assembly).
--   * The frontend adapter assembles the nested `meta` / `config` LobeSession
--     shape from these flat columns (mirrors SessionModel.mapSessionItem).
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
    a.id               AS agent_id,
    a.title            AS agent_title,
    a.description      AS agent_description,
    a.avatar           AS agent_avatar,
    a.background_color AS agent_background_color,
    a.model            AS agent_model,
    a.tags             AS agent_tags,
    a.market_identifier AS agent_market_identifier,
    a.virtual          AS agent_virtual
FROM   sessions s
JOIN   agents_to_sessions ats ON ats.session_id = s.id
JOIN   agents a ON a.id = ats.agent_id
{{- if isSet "workspaceId" }}
WHERE  s.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "s.workspace_id" }}
{{- else }}
WHERE  s.user_id = {{ sqlVal "userId" }} AND s.workspace_id IS NULL
{{- end }}
  AND  s.type = 'agent'
  AND  (
         a.title ILIKE '%' || {{ sqlVal "keyword" }} || '%'
         OR a.description ILIKE '%' || {{ sqlVal "keyword" }} || '%'
       )
ORDER  BY a.id;
