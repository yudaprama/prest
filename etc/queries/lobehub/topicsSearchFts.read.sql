-- topicsSearchFts
-- Replaces: routers/lambda/topic.ts: queryTopics (was BM25 via paradedb)
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   q          (string, required) — search text
--   sessionId  (string, optional) — restrict to a single session
--   agentId    (string, optional) — restrict to topics of this agent
--   page       (int,    default 1)
--   size       (int,    default 20)
--
-- Returns: topics matching the search text, ordered by ts_rank relevance.
--
-- FTS index:  topics_tsv  (migration 0111_add_postgres_fts.sql)
--             GIN(topics_tsv), covers title(A) + content(B) + description(C)
WITH q AS (
    SELECT plainto_tsquery('english', {{ sqlVal "q" }}) AS tsq
)
SELECT
    t.id,
    t.title,
    t.content,
    t.description,
    t.favorite,
    t.session_id,
    t.agent_id,
    t.created_at,
    t.updated_at,
    ts_rank(t.topics_tsv, q.tsq)   AS rank,
    a.title                         AS agent_title,
    a.avatar                        AS agent_avatar,
    a.background_color              AS agent_background_color
FROM topics t, q
LEFT JOIN agents a ON a.id = t.agent_id
WHERE q.tsq <> ''
  AND t.topics_tsv @@ q.tsq
{{- if isSet "workspaceId" }}
  AND  t.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
  AND  t.user_id = {{ sqlVal "userId" }} AND t.workspace_id IS NULL
{{- end }}
{{- if isSet "sessionId" }}
  AND  t.session_id = {{ sqlVal "sessionId" }}
{{- end }}
{{- if isSet "agentId" }}
  AND  t.agent_id = {{ sqlVal "agentId" }}
{{- end }}
ORDER BY rank DESC, t.updated_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
