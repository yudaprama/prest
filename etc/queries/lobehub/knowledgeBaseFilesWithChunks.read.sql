-- knowledgeBaseFilesWithChunks
-- Replaces: routers/lambda/file.ts: getKnowledgeItems
--           repositories/knowledge.ts: query
--
-- Lists files attached to a knowledge base with per-file chunk/embedding
-- status derived from the file's chunk_task_id and embedding_task_id
-- async_tasks rows.
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               tenantId  (optional query param — if set, scope to workspace)

--
-- Query params:
--   knowledgeBaseId (string, optional) — filter to one KB; if omitted,
--                                        returns all files the user has
--                                        attached to any KB
--   page            (int,    default 1)
--   size            (int,    default 50)
SELECT
    f.id,
    f.file_type,
    f.file_hash,
    f.name,
    f.size,
    f.url,
    f.source,
    f.parent_id,
    f.client_id,
    f.metadata,
    kbf.knowledge_base_id,
    COALESCE(cstat.chunk_count, 0)::int        AS chunk_count,
    COALESCE(cstat.total_tokens, 0)::bigint    AS total_tokens,
    ctask.status                               AS chunk_status,
    ctask.error                                AS chunk_error,
    etask.status                               AS embedding_status,
    etask.error                                AS embedding_error,
    f.created_at,
    f.updated_at
FROM   knowledge_base_files kbf
INNER JOIN files f ON f.id = kbf.file_id
LEFT JOIN async_tasks ctask ON ctask.id = f.chunk_task_id
LEFT JOIN async_tasks etask ON etask.id = f.embedding_task_id
LEFT JOIN (
    SELECT fc.file_id,
           COUNT(c.id)                       AS chunk_count,
           COALESCE(SUM(LENGTH(c.text)), 0)::bigint AS total_tokens
    FROM   file_chunks fc
    JOIN   chunks c ON c.id = fc.chunk_id
    {{- if isSet "tenantId" }}
    WHERE  fc.tenant_id = {{ sqlVal "tenantId" }}
    {{- else if eq (defaultOrValue "tenantScope" "") "all" }}
    WHERE  {{ tenantScopeIn "fc.tenant_id" }}
    {{- else }}
    WHERE  fc.tenant_id IS NULL
    {{- end }}
    GROUP  BY fc.file_id
) cstat ON cstat.file_id = f.id
{{- if isSet "tenantId" }}
WHERE  kbf.tenant_id = {{ sqlVal "tenantId" }}
  AND  f.tenant_id   = {{ sqlVal "tenantId" }}
{{- else if eq (defaultOrValue "tenantScope" "") "all" }}
WHERE  {{ tenantScopeIn "kbf.tenant_id" }}
  AND  {{ tenantScopeIn "f.tenant_id" }}
{{- else }}
WHERE  kbf.user_id = {{ sqlVal "userId" }} AND kbf.tenant_id IS NULL
  AND  f.user_id   = {{ sqlVal "userId" }} AND f.tenant_id   IS NULL
{{- end }}
{{- if isSet "knowledgeBaseId" }}
  AND  kbf.knowledge_base_id = {{ sqlVal "knowledgeBaseId" }}
{{- end }}
ORDER  BY f.updated_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "50") }};
