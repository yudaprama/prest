-- taskCreate
-- Replaces: apps/server/src/services/task/index.ts: createTask
--           packages/database/src/models/task.ts: create
--
-- Atomically allocates the next seq within the ownership scope and
-- inserts a new task with a generated identifier (prefix-N).
-- Uses SELECT ... FOR UPDATE to prevent concurrent seq conflicts.
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               tenantId  (optional — if set, scope to workspace)
--               tenantScope (optional "all" — cross-workspace mode)
--
-- Query params:
--   instruction       (text, required)
--   name              (text, optional)
--   description       (text, optional)
--   identifierPrefix  (text, optional, default 'T')
--   assigneeAgentId   (text, optional)
--   assigneeUserId    (text, optional)
--   parentTaskId      (text, optional)
--   priority          (int, optional, default 0)
--   automationMode    (text, optional — 'heartbeat' or 'schedule')
--   schedulePattern   (text, optional)
--   scheduleTimezone  (text, optional, default 'UTC')
--   createdByAgentId  (text, optional)
--   editorData        (jsonb, optional)
WITH scope AS (
    SELECT
        COALESCE(MAX(seq), 0) + 1 AS next_seq
    FROM tasks
    {{- if isSet "tenantId" }}
    WHERE tenant_id = {{ sqlVal "tenantId" }}
    {{- else if eq (defaultOrValue "tenantScope" "") "all" }}
    WHERE {{ tenantScopeIn "tenant_id" }}
    {{- else }}
    WHERE created_by_user_id = {{ sqlVal "userId" }} AND tenant_id IS NULL
    {{- end }}
    FOR UPDATE
),
inserted AS (
    INSERT INTO tasks (
        id, identifier, seq, instruction, name, description,
        assignee_agent_id, assignee_user_id, created_by_user_id,
        created_by_agent_id, parent_task_id, priority,
        automation_mode, schedule_pattern, schedule_timezone,
        editor_data, status, tenant_id
    )
    SELECT
        'task_' || gen_random_uuid()::text,
        COALESCE({{ defaultOrValue "identifierPrefix" "T" }}, 'T') || '-' || s.next_seq,
        s.next_seq,
        {{ sqlVal "instruction" }},
        {{ defaultOrNull "name" }},
        {{ defaultOrNull "description" }},
        {{ defaultOrNull "assigneeAgentId" }},
        {{ defaultOrNull "assigneeUserId" }},
        {{ defaultOrNull "parentTaskId" }},
        COALESCE({{ defaultOrNull "priority" }}::int, 0),
        {{ defaultOrNull "automationMode" }},
        {{ defaultOrNull "schedulePattern" }},
        COALESCE({{ defaultOrNull "scheduleTimezone" }}, 'UTC'),
        {{ defaultOrNull "editorData" }}::jsonb,
        'backlog',
        {{ defaultOrNull "tenantId" }}
    FROM scope s
    RETURNING *
)
SELECT
    id,
    identifier,
    seq,
    instruction,
    name,
    description,
    status,
    priority,
    assignee_agent_id AS "assigneeAgentId",
    assignee_user_id  AS "assigneeUserId",
    created_by_user_id AS "createdByUserId",
    parent_task_id   AS "parentTaskId",
    tenant_id     AS "tenantId",
    automation_mode  AS "automationMode",
    schedule_pattern AS "schedulePattern",
    schedule_timezone AS "scheduleTimezone",
    config,
    error,
    total_topics     AS "totalTopics",
    created_at       AS "createdAt",
    updated_at       AS "updatedAt"
FROM inserted;
