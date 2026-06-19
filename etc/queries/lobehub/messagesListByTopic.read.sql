-- messagesListByTopic (full version, also known as messagesWithPluginsByTopic)
-- Replaces: routers/lambda/message.ts: getMessages
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — if set, scope to workspace)
--               workspaceScope (optional "all" — cross-workspace mode;
--                               resolved via Keto membership; else personal
--                               scope with workspace_id IS NULL)
--
-- Query params:
--   topicId  (string, required)
--   groupId  (string, optional) — further filter to one message group
--   page     (int,    default 1)
--   size     (int,    default 50)
--
-- Returns: array of messages with nested json_agg of translates, plugins,
--          tts metadata, and any files/chunks referenced.
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
         {{- if isSet "workspaceId" }}
         WHERE  mt.id = m.id AND mt.workspace_id = {{ sqlVal "workspaceId" }}
         {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
         WHERE  mt.id = m.id AND {{ workspaceScopeIn "mt.workspace_id" }}
         {{- else }}
         WHERE  mt.id = m.id
         {{- end }}),
        '[]'::json
    ) AS translates,
    COALESCE(
        (SELECT json_agg(json_build_object('id', mp.id, 'tool_call_id', mp.tool_call_id, 'type', mp.type, 'api_name', mp.api_name, 'identifier', mp.identifier, 'arguments', mp.arguments, 'state', mp.state, 'error', mp.error, 'intervention', mp.intervention))
         FROM   message_plugins mp
         {{- if isSet "workspaceId" }}
         WHERE  mp.id = m.id AND mp.workspace_id = {{ sqlVal "workspaceId" }}
         {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
         WHERE  mp.id = m.id AND {{ workspaceScopeIn "mp.workspace_id" }}
         {{- else }}
         WHERE  mp.id = m.id
         {{- end }}),
        '[]'::json
    ) AS plugins,
    COALESCE(
        (SELECT json_agg(json_build_object('id', mtt.id, 'voice', mtt.voice, 'file_id', mtt.file_id, 'content_md5', mtt.content_md5))
         FROM   message_tts mtt
         {{- if isSet "workspaceId" }}
         WHERE  mtt.id = m.id AND mtt.workspace_id = {{ sqlVal "workspaceId" }}
         {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
         WHERE  mtt.id = m.id AND {{ workspaceScopeIn "mtt.workspace_id" }}
         {{- else }}
         WHERE  mtt.id = m.id
         {{- end }}),
        '[]'::json
    ) AS tts,
    COALESCE(
        (SELECT json_agg(json_build_object('id', f.id, 'name', f.name, 'url', f.url, 'file_type', f.file_type, 'size', f.size))
         FROM   messages_files mf
         JOIN   files f ON f.id = mf.file_id
         WHERE  mf.message_id = m.id),
        '[]'::json
    ) AS files
FROM   messages m
{{- if isSet "workspaceId" }}
WHERE  m.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "m.workspace_id" }}
{{- else }}
WHERE  m.user_id = {{ sqlVal "userId" }} AND m.workspace_id IS NULL
{{- end }}
  AND  m.topic_id = {{ sqlVal "topicId" }}
{{- if isSet "groupId" }}
  AND  m.message_group_id = {{ sqlVal "groupId" }}
{{- end }}
ORDER  BY m.created_at ASC, m.id ASC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "50") }};
