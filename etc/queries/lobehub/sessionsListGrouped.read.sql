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
LEFT JOIN (
    SELECT session_id, COUNT(*) AS cnt
    FROM   topics
    WHERE  user_id = {{ sqlVal "userId" }}
    GROUP  BY session_id
) t ON t.session_id = s.id
WHERE  s.user_id = {{ sqlVal "userId" }}
{{- if eq (defaultOrValue "includePinnedOnly" "false") "true" }}
  AND  s.pinned = true
{{- end }}
ORDER  BY g.sort NULLS LAST, s.pinned DESC, s.updated_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
