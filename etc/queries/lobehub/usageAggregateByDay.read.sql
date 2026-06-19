-- usageAggregateByDay
-- Replaces: services/usage/index.ts: findByDateRange
--
-- Soft migration: re-aggregates token/cost from messages.usage jsonb in the
-- LobeHub DB. The authoritative source is Plano's conversation_states
-- (Yarsew DB) populated by the LLM path. Use this template as a quick local
-- read; for billing, hit Plano directly.
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — if set, scope to workspace)
--               workspaceScope (optional "all" — cross-workspace mode,
--                               resolved via Keto membership; else personal
--                               scope with workspace_id IS NULL)
--
-- Query params:
--   startDate (timestamp, required)
--   endDate   (timestamp, required)
--   model     (string,    optional)
--   provider  (string,    optional)
SELECT
    date_trunc('day', m.created_at)             AS day,
    COUNT(*)                                    AS message_count,
    COUNT(DISTINCT m.topic_id)                  AS active_topics,
    COALESCE(SUM((m.usage->>'inputTokens')::bigint),  0)::bigint AS input_tokens,
    COALESCE(SUM((m.usage->>'outputTokens')::bigint), 0)::bigint AS output_tokens,
    COALESCE(SUM((m.usage->>'totalTokens')::bigint),  0)::bigint AS total_tokens,
    COALESCE(SUM((m.usage->>'cost')::numeric),         0)::numeric AS total_cost,
    COALESCE(SUM((m.usage->>'cost')::numeric) FILTER (WHERE m.role = 'user'),
             0)::numeric                                        AS user_cost,
    COALESCE(SUM((m.usage->>'cost')::numeric) FILTER (WHERE m.role = 'assistant'),
             0)::numeric                                        AS assistant_cost
FROM   messages m
{{- if isSet "workspaceId" }}
WHERE  m.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "m.workspace_id" }}
{{- else }}
WHERE  m.user_id = {{ sqlVal "userId" }} AND m.workspace_id IS NULL
{{- end }}
  AND  m.role IN ('user', 'assistant')
  AND  m.created_at >= {{ sqlVal "startDate" }}
  AND  m.created_at <  {{ sqlVal "endDate" }}
{{- if isSet "model" }}
  AND  m.model = {{ sqlVal "model" }}
{{- end }}
{{- if isSet "provider" }}
  AND  m.provider = {{ sqlVal "provider" }}
{{- end }}
GROUP  BY day
ORDER  BY day ASC;
