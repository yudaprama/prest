-- connectorsListWithTools
-- Replaces: routers/lambda/connector.ts:list
--           services/connector.ts:list (was lambdaClient.connector.list)
--
-- Returns every connector for the caller with its tools nested as a jsonb
-- array — matching the TS list() shape ({ ...connector, oidcConfig, tools }).
-- Secrets are stripped:
--   * `credentials` (encrypted OAuth/token blob) is omitted entirely.
--   * `oidc_config.clientSecret` is removed; the rest of oidc_config (endpoints,
--     clientId, scopes) is safe to ship to the browser.
--
-- Auth scope: userId      (auto-injected from Kratos identity)
--             workspaceId (optional query param — scope to workspace;
--                          else personal scope with workspace_id IS NULL)
--
-- Response keys are snake_case; LobehubClient (camelCase: true) rewrites them
-- recursively, including the nested `tools` jsonb, so the FE sees camelCase.
SELECT
    c.id,
    c.user_id,
    c.workspace_id,
    c.identifier,
    c.name,
    c.source_type,
    c.mcp_server_url,
    c.mcp_connection_type,
    c.mcp_stdio_config,
    c.status,
    c.is_enabled,
    c.token_expires_at,
    c.metadata,
    c.created_at,
    c.updated_at,
    CASE
        WHEN c.oidc_config IS NULL THEN NULL
        ELSE (c.oidc_config - 'clientSecret'::text)
    END AS oidc_config,
    COALESCE(
        (
            SELECT jsonb_agg(jsonb_build_object(
                'id', t.id,
                'user_connector_id', t.user_connector_id,
                'tool_name', t.tool_name,
                'display_name', t.display_name,
                'description', t.description,
                'input_schema', t.input_schema,
                'output_schema', t.output_schema,
                'crud_type', t.crud_type,
                'render_config', t.render_config,
                'permission', t.permission,
                'is_work_artifact', t.is_work_artifact,
                'work_artifact_config', t.work_artifact_config,
                'limit_config', t.limit_config,
                'metadata', t.metadata,
                'created_at', t.created_at,
                'updated_at', t.updated_at
            ) ORDER BY t.created_at, t.id)
            FROM user_connector_tools t
            WHERE t.user_connector_id = c.id
        ),
        '[]'::jsonb
    ) AS tools
FROM user_connectors c
{{- if isSet "workspaceId" }}
WHERE  c.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  c.user_id = {{ sqlVal "userId" }} AND c.workspace_id IS NULL
{{- end }}
ORDER  BY c.created_at, c.id;
