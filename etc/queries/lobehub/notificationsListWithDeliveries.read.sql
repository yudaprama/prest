SELECT
    n.id,
    n.category,
    n.type,
    n.title,
    n.content,
    n.dedupe_key,
    n.action_url,
    n.is_read,
    n.is_archived,
    n.created_at,
    n.updated_at,
    COALESCE(
        (SELECT json_agg(json_build_object(
                    'id',              d.id,
                    'channel',         d.channel,
                    'status',          d.status,
                    'provider_message_id', d.provider_message_id,
                    'failed_reason',   d.failed_reason,
                    'sent_at',         d.sent_at))
         FROM   notification_deliveries d
         WHERE  d.notification_id = n.id),
        '[]'::json
    ) AS deliveries
FROM   notifications n
WHERE  n.user_id = {{ sqlVal "userId" }}
{{- if eq (defaultOrValue "unreadOnly" "false") "true" }}
  AND  n.is_read      = false
{{- end }}
{{- if eq (defaultOrValue "activeOnly" "true") "true" }}
  AND  n.is_archived  = false
{{- end }}
{{- if isSet "category" }}
  AND  n.category = {{ sqlVal "category" }}
{{- end }}
{{- if isSet "type" }}
  AND  n.type = {{ sqlVal "type" }}
{{- end }}
ORDER  BY n.created_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
