-- messageWordCount
-- Replaces: packages/database/src/models/message.ts: countWords
--
-- Sum of content length (character count) for user's messages.
-- Optional date range filter.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   startDate (timestamp, optional)
--   endDate   (timestamp, optional)
SELECT
    COALESCE(SUM(LENGTH(m.content)), 0)::bigint AS count
FROM   messages m
WHERE  m.user_id = {{ sqlVal "userId" }}
{{- if isSet "startDate" }}
  AND  m.created_at >= {{ sqlVal "startDate" }}
{{- end }}
{{- if isSet "endDate" }}
  AND  m.created_at <= {{ sqlVal "endDate" }}
{{- end }};
