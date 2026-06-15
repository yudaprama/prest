-- generationBatchesWithGenerations
-- Replaces: routers/lambda/generationBatch.ts: getGenerationBatches
--           models/generationBatch.ts: findByTopicIdWithGenerations
--
-- Each batch nests its child generations; each child nests its async task.
-- Implemented as one outer SELECT with a json_agg subquery that itself does a
-- LEFT JOIN to async_tasks. Single round-trip, no Drizzle `with:` required.
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   topicId (string, required)
--   type    (string, optional) — currently 'video' adds an `avgLatencyMs` from
--                                the generation model. The plain SELECT below
--                                returns the per-generation latency; the
--                                caller can compute the average client-side.
--   page    (int,    default 1)
--   size    (int,    default 20)
--
-- Returns: array of generation_batches with nested `generations[]` (each with
--          its async_task embedded).
SELECT
    gb.id,
    gb.generation_topic_id,
    gb.provider,
    gb.model,
    gb.prompt,
    gb.width,
    gb.height,
    gb.ratio,
    gb.config,
    gb.created_at,
    gb.updated_at,
    COALESCE(
        (SELECT json_agg(
                  json_build_object(
                      'id',             g.id,
                      'generation_batch_id', g.generation_batch_id,
                      'async_task_id',  g.async_task_id,
                      'file_id',        g.file_id,
                      'seed',           g.seed,
                      'asset',          g.asset,
                      'created_at',     g.created_at,
                      'updated_at',     g.updated_at,
                      'async_task',     jsonb_build_object(
                          'id',          at.id,
                          'type',        at.type,
                          'status',      at.status,
                          'error',       at.error,
                          'duration',    at.duration,
                          'created_at',  at.created_at,
                          'updated_at',  at.updated_at
                      )
                  )
                  ORDER BY g.created_at ASC, g.id ASC)
         FROM   generations g
         LEFT JOIN async_tasks at ON at.id = g.async_task_id
         WHERE  g.generation_batch_id = gb.id
         {{- if isSet "workspaceId" }}
         AND    g.workspace_id = {{ sqlVal "workspaceId" }}
         {{- else }}
         AND    g.workspace_id IS NULL
         {{- end }}),
        '[]'::json
    ) AS generations
FROM   generation_batches gb
{{- if isSet "workspaceId" }}
WHERE  gb.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  gb.user_id = {{ sqlVal "userId" }} AND gb.workspace_id IS NULL
{{- end }}
  AND  gb.generation_topic_id = {{ sqlVal "topicId" }}
ORDER  BY gb.created_at ASC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
