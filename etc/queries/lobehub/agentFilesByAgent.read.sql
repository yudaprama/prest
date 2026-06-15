-- agentFilesByAgent
-- Replaces: services/agentDocuments/index.ts: findByAgent
-- Replaces: routers/lambda/agentDocument.ts: getDocuments / listDocuments
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   agentId  (string, required)
--   page     (int,    default 1)
--   size     (int,    default 20)
--
-- Returns: array of documents attached to one agent, with the join row
--          (load rule, policy, access bitmask) embedded as a `settings` object.
SELECT
    d.id,
    d.title,
    d.description,
    d.file_type,
    d.filename,
    d.source_type,
    d.source,
    d.file_id,
    d.knowledge_base_id,
    d.parent_id,
    d.client_id,
    d.slug,
    d.metadata,
    d.editor_data,
    d.total_char_count,
    d.total_line_count,
    d.user_id,
    d.workspace_id,
    d.created_at,
    d.updated_at,
    jsonb_build_object(
        'id',              ad.id,
        'template_id',     ad.template_id,
        'access_self',     ad.access_self,
        'access_shared',   ad.access_shared,
        'access_public',   ad.access_public,
        'policy_load',     ad.policy_load,
        'policy',          ad.policy,
        'policy_load_position', ad.policy_load_position,
        'policy_load_format',   ad.policy_load_format,
        'policy_load_rule',     ad.policy_load_rule
    ) AS settings
FROM   documents d
INNER JOIN agent_documents ad
        ON ad.document_id = d.id
        AND ad.deleted_at IS NULL
        {{- if isSet "workspaceId" }}
        AND ad.workspace_id = {{ sqlVal "workspaceId" }}
        {{- else }}
        AND ad.workspace_id IS NULL
        {{- end }}
{{- if isSet "workspaceId" }}
WHERE  d.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  d.user_id = {{ sqlVal "userId" }} AND d.workspace_id IS NULL
{{- end }}
  AND  ad.agent_id = {{ sqlVal "agentId" }}
ORDER  BY d.updated_at ASC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
