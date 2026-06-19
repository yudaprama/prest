-- recentByUser
-- Replaces: routers/lambda/recent.ts: queryRecent
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--              workspaceId (optional query param — if set, scope to workspace;
--                           else personal scope with workspace_id IS NULL)
--
-- Query params:
--   limit  (int, default 10) — max rows to return
--
-- Returns: UNION of `topic`, `document`, and `task` rows ordered by updated_at DESC.
--          Mirrors the `RecentModel.queryRecent` Drizzle query in
--          packages/database/src/models/recent.ts.
--
-- Notes:
--   * `tasks` uses `createdByUserId` (not `userId`) so the WHERE branch is
--     inline (matches the TS implementation).
--   * `task_final_statuses` ('completed', 'canceled') are excluded to avoid
--     cluttering the Recent sidebar.
--   * `system_topic_triggers` ('cron', 'eval', 'task_manager', 'task') are
--     excluded — those topics live in their own surfaces.
--   * `tool_document_source_types` ('agent', 'agent-signal', 'file', 'web')
--     are excluded — only user-authored ('api') and legacy 'topic' rows
--     remain.
--   * Topic arm requires topics whose group is non-null OR whose agent is
--     'inbox' OR whose agent is non-virtual and groupless.

WITH latest_topic_message AS (
    SELECT
        m.topic_id,
        MAX(m.updated_at) AS message_at
    FROM   messages m
    {{- if isSet "workspaceId" }}
    WHERE  m.workspace_id = {{ sqlVal "workspaceId" }}
    {{- else }}
    WHERE  m.user_id = {{ sqlVal "userId" }} AND m.workspace_id IS NULL
    {{- end }}
    GROUP  BY m.topic_id
),
topic_arm AS (
    SELECT
        t.id,
        t.metadata,
        t.group_id    AS route_group_id,
        t.agent_id    AS route_id,
        NULL::text    AS status,
        COALESCE(t.title, 'Untitled Topic') AS title,
        'topic'::text AS type,
        COALESCE(ltm.message_at, t.updated_at) AS updated_at
    FROM   topics t
    LEFT JOIN agents a ON a.id = t.agent_id
    LEFT JOIN latest_topic_message ltm ON ltm.topic_id = t.id
    {{- if isSet "workspaceId" }}
    WHERE  t.workspace_id = {{ sqlVal "workspaceId" }}
    {{- else }}
    WHERE  t.user_id = {{ sqlVal "userId" }} AND t.workspace_id IS NULL
    {{- end }}
      AND  (
            t.group_id IS NOT NULL
         OR a.slug = 'inbox'
         OR (t.group_id IS NULL AND (a.virtual IS NULL OR a.virtual = false))
      )
      AND  (t.trigger IS NULL OR t.trigger NOT IN ('cron', 'eval', 'task_manager', 'task'))
),
document_arm AS (
    SELECT
        d.id,
        NULL::jsonb   AS metadata,
        NULL::text    AS route_group_id,
        NULL::text    AS route_id,
        NULL::text    AS status,
        COALESCE(d.title, d.filename, 'Untitled Document') AS title,
        'document'::text AS type,
        d.updated_at
    FROM   documents d
    {{- if isSet "workspaceId" }}
    WHERE  d.workspace_id = {{ sqlVal "workspaceId" }}
    {{- else }}
    WHERE  d.user_id = {{ sqlVal "userId" }} AND d.workspace_id IS NULL
    {{- end }}
      AND  d.source_type NOT IN ('agent', 'agent-signal', 'file', 'web')
      AND  d.knowledge_base_id IS NULL
      AND  d.file_type <> 'custom/folder'
),
task_arm AS (
    SELECT
        tk.id,
        NULL::jsonb   AS metadata,
        NULL::text    AS route_group_id,
        tk.assignee_agent_id AS route_id,
        tk.status,
        COALESCE(tk.name, tk.instruction, 'Untitled Task') AS title,
        'task'::text  AS type,
        tk.updated_at
    FROM   tasks tk
    {{- if isSet "workspaceId" }}
    WHERE  tk.workspace_id = {{ sqlVal "workspaceId" }}
    {{- else }}
    WHERE  tk.created_by_user_id = {{ sqlVal "userId" }} AND tk.workspace_id IS NULL
    {{- end }}
      AND  tk.status NOT IN ('completed', 'canceled')
)
SELECT * FROM topic_arm
UNION ALL
SELECT * FROM document_arm
UNION ALL
SELECT * FROM task_arm
ORDER  BY updated_at DESC
LIMIT  {{ sqlVal "limit" }};
