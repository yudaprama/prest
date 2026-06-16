-- messagesSearchFts
-- Replaces: routers/lambda/message.ts: searchMessages (was BM25 via paradedb)
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   q          (string, required) — search text, e.g. "hello world from lobehub"
--   topicId    (string, optional) — restrict to a single topic
--   agentId    (string, optional) — restrict to messages of this agent
--   page       (int,    default 1)
--   size       (int,    default 20)
--
-- Returns: messages matching the search text, ordered by ts_rank relevance.
--          The `rank` field exposes the raw ts_rank score (0.0–17.0 typical).
--
-- FTS index:  messages_tsv  (migration 0111_add_postgres_fts.sql)
--             GIN(messages_tsv), covers content(B) + summary(C)
--
-- Example:
--   GET /_QUERIES/lobehub/messagesSearchFts?q=deploy+failed&size=10
--   GET /_QUERIES/lobehub/messagesSearchFts?q=hello&topicId=abc123
WITH q AS (
    SELECT plainto_tsquery('english', {{ sqlVal "q" }}) AS tsq
)
SELECT
    m.id,
    m.role,
    m.content,
    m.summary,
    m.model,
    m.provider,
    m.topic_id,
    m.agent_id,
    m.created_at,
    m.updated_at,
    ts_rank(m.messages_tsv, q.tsq)   AS rank
FROM messages m, q
WHERE q.tsq <> ''                        -- empty query → no rows (safety)
  AND m.messages_tsv @@ q.tsq
{{- if isSet "workspaceId" }}
  AND  m.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
  AND  m.user_id = {{ sqlVal "userId" }} AND m.workspace_id IS NULL
{{- end }}
{{- if isSet "topicId" }}
  AND  m.topic_id = {{ sqlVal "topicId" }}
{{- end }}
{{- if isSet "agentId" }}
  AND  m.agent_id = {{ sqlVal "agentId" }}
{{- end }}
  AND  m.role <> 'tool'                  -- skip tool-call payloads
ORDER BY rank DESC, m.created_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
