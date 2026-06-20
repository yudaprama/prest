-- knowledgeItemStatuses
-- Replaces: routers/lambda/file.ts: getKnowledgeItemStatusesByIds
--           routers/lambda/file.ts: getKnowledgeItemStatusMap (read path)
--
-- Returns per-file chunk/embedding status for a set of file IDs.
-- Joins files with file_chunks (chunk count) and async_tasks
-- (chunking + embedding task status/error).
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — if set, scope to workspace)
--               workspaceScope (optional "all" — cross-workspace mode;
--                               resolved via Keto membership; else personal
--                               scope with workspace_id IS NULL)
--
-- Query params:
--   ids (string, required) — comma-separated file IDs
SELECT
    f.id,
    COALESCE(cc.chunk_count, 0)::int       AS "chunkCount",
    ctask.status                            AS "chunkingStatus",
    ctask.error                             AS "chunkingError",
    etask.status                            AS "embeddingStatus",
    etask.error                             AS "embeddingError",
    (etask.status = 'success')              AS "finishEmbedding"
FROM   files f
LEFT JOIN async_tasks ctask ON ctask.id = f.chunk_task_id
LEFT JOIN async_tasks etask ON etask.id = f.embedding_task_id
LEFT JOIN (
    SELECT fc.file_id,
           COUNT(fc.chunk_id)::int AS chunk_count
    FROM   file_chunks fc
    {{- if isSet "workspaceId" }}
    WHERE  fc.workspace_id = {{ sqlVal "workspaceId" }}
    {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
    WHERE  {{ workspaceScopeIn "fc.workspace_id" }}
    {{- else }}
    WHERE  fc.workspace_id IS NULL
    {{- end }}
    GROUP  BY fc.file_id
) cc ON cc.file_id = f.id
{{- if isSet "workspaceId" }}
WHERE  f.workspace_id = {{ sqlVal "workspaceId" }}
{{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
WHERE  {{ workspaceScopeIn "f.workspace_id" }}
{{- else }}
WHERE  f.user_id = {{ sqlVal "userId" }} AND f.workspace_id IS NULL
{{- end }}
  AND  f.id IN {{ sqlList "ids" }}
ORDER  BY f.id;
