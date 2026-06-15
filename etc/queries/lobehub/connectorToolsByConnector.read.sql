-- connectorToolsByConnector
-- Replaces: routers/lambda/connector.ts: list (the tools sublist)
--           services/connector/impls/...: queryByConnector
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   userConnectorId (uuid, required) — the connector row whose tools we want
--   includeDisabled (bool, default false) — by default, drop permission='disabled'
--                                            rows so the agent runtime never sees
--                                            blocked tools.
--   page            (int,  default 1)
--   size            (int,  default 50)
--
-- Returns: array of user_connector_tools rows for one connector.
SELECT
    id,
    user_connector_id,
    tool_name,
    display_name,
    description,
    input_schema,
    output_schema,
    crud_type,
    render_config,
    permission,
    is_work_artifact,
    work_artifact_config,
    limit_config,
    metadata,
    created_at,
    updated_at
FROM   user_connector_tools
{{- if isSet "workspaceId" }}
WHERE  workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  user_id = {{ sqlVal "userId" }} AND workspace_id IS NULL
{{- end }}
  AND  user_connector_id = {{ sqlVal "userConnectorId" }}
{{- if eq (defaultOrValue "includeDisabled" "false") "false" }}
  AND  permission <> 'disabled'
{{- end }}
ORDER  BY created_at ASC, id ASC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "50") }};
