-- knowledgeItemStatuses
-- Replaces: routers/lambda/file.ts: getKnowledgeItemStatusesByIds
--           routers/lambda/file.ts: getKnowledgeItemStatusMap (read path)
--
-- Returns per-file chunk/embedding status for a set of file IDs.
-- Joins files with file_chunks (chunk count) and async_tasks
-- (chunking + embedding task status/error).
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               tenantId  (optional query param — if set, scope to workspace)

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
    {{- if isSet "tenantId" }}
    WHERE  fc.tenant_id = {{ sqlVal "tenantId" }}
    {{- else }}
    WHERE  fc.tenant_id IS NULL
    {{- end }}
    GROUP  BY fc.file_id
) cc ON cc.file_id = f.id
{{- if isSet "tenantId" }}
WHERE  f.tenant_id = {{ sqlVal "tenantId" }}
{{- else }}
WHERE  f.user_id = {{ sqlVal "userId" }} AND f.tenant_id IS NULL
{{- end }}
  AND  f.id IN {{ sqlList "ids" }}
ORDER  BY f.id;
