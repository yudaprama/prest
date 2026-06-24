-- agentsCount
-- Replaces: apps/server router agent.countAgents
--
-- Counts non-virtual agents for the authenticated user.
-- Uses the same scoping rules as agentsListWithStats.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   keyword         (string, optional) — fuzzy match against title/description
--   sessionGroupId  (string, optional) — filter by session group
SELECT COUNT(*)::int AS count
FROM agents a
WHERE a.user_id = {{ sqlVal "userId" }}
  AND a.workspace_id IS NULL
  AND (a.virtual = false OR a.virtual IS NULL)
{{- if isSet "sessionGroupId" }}
  AND a.session_group_id = {{ sqlVal "sessionGroupId" }}
{{- end }}
{{- if isSet "keyword" }}
  AND (a.title ILIKE '%' || {{ sqlVal "keyword" }} || '%'
       OR a.description ILIKE '%' || {{ sqlVal "keyword" }} || '%')
{{- end }};
