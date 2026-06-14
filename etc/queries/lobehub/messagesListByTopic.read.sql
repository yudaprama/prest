SELECT
    m.id,
    m.role,
    m.content,
    m.editor_data,
    m.summary,
    m.reasoning,
    m.search,
    m.metadata,
    m.usage,
    m.model,
    m.provider,
    m.favorite,
    m.error,
    m.tools,
    m.client_id,
    m.parent_id,
    m.message_group_id,
    m.quota_id,
    m.target_id,
    m.created_at,
    m.updated_at,
    COALESCE(
        (SELECT json_agg(json_build_object('id', mt.id, 'from', mt."from", 'to', mt."to", 'content', mt.content))
         FROM   message_translates mt
         WHERE  mt.id = m.id),
        '[]'::json
    ) AS translates,
    COALESCE(
        (SELECT json_agg(json_build_object('id', mp.id, 'tool_call_id', mp.tool_call_id, 'type', mp.type, 'api_name', mp.api_name, 'identifier', mp.identifier, 'arguments', mp.arguments, 'state', mp.state, 'error', mp.error))
         FROM   message_plugins mp
         WHERE  mp.id = m.id),
        '[]'::json
    ) AS plugins
FROM   messages m
WHERE  m.user_id  = {{ sqlVal "userId" }}
  AND  m.topic_id = {{ sqlVal "topicId" }}
{{- if isSet "groupId" }}
  AND  m.message_group_id = {{ sqlVal "groupId" }}
{{- end }}
ORDER  BY m.created_at ASC, m.id ASC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "50") }};
