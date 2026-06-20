-- knowledgeItemsList
-- Replaces: routers/lambda/file.ts: getKnowledgeItems
--           repositories/knowledge.ts: query
--
-- Lists knowledge items (files + documents) with per-file chunk/embedding
-- status. Handles both knowledge-base-scoped and personal/inbox views via
-- conditional branches on knowledgeBaseId.
--
-- Auth scope:   userId       (auto-injected from Kratos identity)
--               workspaceId  (optional query param — if set, scope to workspace)
--               workspaceScope (optional "all" — cross-workspace mode;
--                               resolved via Keto membership; else personal
--                               scope with workspace_id IS NULL)
--
-- Query params:
--   knowledgeBaseId          (string, optional) — scope to one KB
--   category                 (string, optional) — all/audios/documents/images/videos/websites
--   q                        (string, optional) — search keyword
--   parentId                 (string, optional) — parent folder/document ID
--   showFilesInKnowledgeBase (bool,   default false) — include KB files in inbox
--   sorter                   (string, optional) — createdAt/size/name/updatedAt
--   sortType                 (string, optional) — asc/desc (default desc)
--   page                     (int,    default 1)
--   size                     (int,    default 50)
--
-- Returns: rows with id, file_id, document_id, name, file_type, size, url,
--          created_at, updated_at, chunk_task_id, embedding_task_id,
--          editor_data, content, slug, metadata, source_type,
--          chunk_count, chunking_status, chunking_error,
--          embedding_status, embedding_error, finish_embedding.

SELECT * FROM (
{{- if isSet "knowledgeBaseId" }}
-- ═══════════════════════════════════════════════════════════
-- KB-scoped: files linked to knowledgeBaseId + standalone docs
-- ═══════════════════════════════════════════════════════════
SELECT * FROM (
  -- KB files
  SELECT
      COALESCE(d.id, f.id)           AS id,
      f.id                           AS file_id,
      d.id                           AS document_id,
      f.name,
      f.file_type,
      f.size,
      f.url,
      f.created_at,
      f.updated_at,
      f.chunk_task_id,
      f.embedding_task_id,
      d.editor_data,
      d.content,
      d.slug,
      COALESCE(d.metadata, f.metadata) AS metadata,
      'file'                         AS source_type,
      COALESCE(cc.chunk_count, 0)::int AS chunk_count,
      ctask.status                   AS chunking_status,
      ctask.error                    AS chunking_error,
      etask.status                   AS embedding_status,
      etask.error                    AS embedding_error,
      (etask.status = 'success')     AS finish_embedding
  FROM   files f
  INNER JOIN knowledge_base_files kbf
      ON f.id = kbf.file_id AND kbf.knowledge_base_id = {{ sqlVal "knowledgeBaseId" }}
  LEFT JOIN documents d ON f.id = d.file_id
  LEFT JOIN async_tasks ctask ON ctask.id = f.chunk_task_id
  LEFT JOIN async_tasks etask ON etask.id = f.embedding_task_id
  LEFT JOIN (
      SELECT fc.file_id, COUNT(fc.chunk_id)::int AS chunk_count
      FROM file_chunks fc
      GROUP BY fc.file_id
  ) cc ON cc.file_id = f.id
  {{- if isSet "workspaceId" }}
  WHERE  f.workspace_id = {{ sqlVal "workspaceId" }}
  {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
  WHERE  {{ workspaceScopeIn "f.workspace_id" }}
  {{- else }}
  WHERE  f.user_id = {{ sqlVal "userId" }} AND f.workspace_id IS NULL
  {{- end }}
  {{- if isSet "parentId" }}
    {{- if eq (defaultOrValue "parentId" "") "null" }}
    AND  f.parent_id IS NULL
    {{- else }}
    AND  f.parent_id = {{ sqlVal "parentId" }}
    {{- end }}
  {{- end }}
  {{- if isSet "q" }}
    AND  f.name ILIKE {{ sqlVal "qPattern" }}
  {{- end }}
  {{- if isSet "category" }}
    {{- if eq (defaultOrValue "category" "all") "audios" }}
    AND  f.file_type ILIKE 'audio%'
    {{- else if eq (defaultOrValue "category" "all") "documents" }}
    AND  (f.file_type ILIKE 'application%' OR f.file_type ILIKE 'custom%')
    {{- else if eq (defaultOrValue "category" "all") "images" }}
    AND  f.file_type ILIKE 'image%'
    {{- else if eq (defaultOrValue "category" "all") "videos" }}
    AND  f.file_type ILIKE 'video%'
    {{- else if eq (defaultOrValue "category" "all") "websites" }}
    AND  f.file_type ILIKE 'text/html%'
    {{- end }}
  {{- end }}

  UNION ALL

  -- KB standalone documents (folders/notes without a linked file)
  SELECT
      d.id,
      d.file_id,
      d.id                           AS document_id,
      COALESCE(d.title, d.filename, 'Untitled') AS name,
      d.file_type,
      d.total_char_count             AS size,
      d.source                       AS url,
      d.created_at,
      d.updated_at,
      NULL::uuid                     AS chunk_task_id,
      NULL::uuid                     AS embedding_task_id,
      d.editor_data,
      d.content,
      d.slug,
      d.metadata,
      'document'                     AS source_type,
      NULL::int                      AS chunk_count,
      NULL::text                     AS chunking_status,
      NULL::jsonb                    AS chunking_error,
      NULL::text                     AS embedding_status,
      NULL::jsonb                    AS embedding_error,
      false                          AS finish_embedding
  FROM   documents d
  {{- if isSet "workspaceId" }}
  WHERE  d.workspace_id = {{ sqlVal "workspaceId" }}
  {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
  WHERE  {{ workspaceScopeIn "d.workspace_id" }}
  {{- else }}
  WHERE  d.user_id = {{ sqlVal "userId" }} AND d.workspace_id IS NULL
  {{- end }}
    AND  d.source_type != 'file'
    AND  d.file_id IS NULL
    AND  d.knowledge_base_id = {{ sqlVal "knowledgeBaseId" }}
  {{- if isSet "parentId" }}
    {{- if eq (defaultOrValue "parentId" "") "null" }}
    AND  d.parent_id IS NULL
    {{- else }}
    AND  d.parent_id = {{ sqlVal "parentId" }}
    {{- end }}
  {{- end }}
  {{- if isSet "q" }}
    AND  (d.title ILIKE {{ sqlVal "qPattern" }} OR d.filename ILIKE {{ sqlVal "qPattern" }})
  {{- end }}
  {{- if isSet "category" }}
    {{- if eq (defaultOrValue "category" "all") "audios" }}
    AND  false
    {{- else if eq (defaultOrValue "category" "all") "documents" }}
    AND  (d.file_type ILIKE 'application%' OR (d.file_type ILIKE 'custom%' AND d.file_type != 'custom/document'))
    {{- else if eq (defaultOrValue "category" "all") "images" }}
    AND  false
    {{- else if eq (defaultOrValue "category" "all") "videos" }}
    AND  false
    {{- else if eq (defaultOrValue "category" "all") "websites" }}
    AND  false
    {{- end }}
  {{- end }}
) kb_combined

{{- else }}
-- ═══════════════════════════════════════════════════════════
-- Personal/inbox: all user files + documents (no KB scope)
-- ═══════════════════════════════════════════════════════════
SELECT * FROM (
  -- Personal files
  SELECT
      COALESCE(d.id, f.id)           AS id,
      f.id                           AS file_id,
      d.id                           AS document_id,
      f.name,
      f.file_type,
      f.size,
      f.url,
      f.created_at,
      f.updated_at,
      f.chunk_task_id,
      f.embedding_task_id,
      d.editor_data,
      d.content,
      d.slug,
      COALESCE(d.metadata, f.metadata) AS metadata,
      'file'                         AS source_type,
      COALESCE(cc.chunk_count, 0)::int AS chunk_count,
      ctask.status                   AS chunking_status,
      ctask.error                    AS chunking_error,
      etask.status                   AS embedding_status,
      etask.error                    AS embedding_error,
      (etask.status = 'success')     AS finish_embedding
  FROM   files f
  LEFT JOIN documents d ON f.id = d.file_id
  LEFT JOIN async_tasks ctask ON ctask.id = f.chunk_task_id
  LEFT JOIN async_tasks etask ON etask.id = f.embedding_task_id
  LEFT JOIN (
      SELECT fc.file_id, COUNT(fc.chunk_id)::int AS chunk_count
      FROM file_chunks fc
      GROUP BY fc.file_id
  ) cc ON cc.file_id = f.id
  {{- if isSet "workspaceId" }}
  WHERE  f.workspace_id = {{ sqlVal "workspaceId" }}
  {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
  WHERE  {{ workspaceScopeIn "f.workspace_id" }}
  {{- else }}
  WHERE  f.user_id = {{ sqlVal "userId" }} AND f.workspace_id IS NULL
  {{- end }}
  {{- if not (eq (defaultOrValue "showFilesInKnowledgeBase" "false") "true") }}
    AND  NOT EXISTS (SELECT 1 FROM knowledge_base_files kbf2 WHERE kbf2.file_id = f.id)
  {{- end }}
  {{- if isSet "parentId" }}
    {{- if eq (defaultOrValue "parentId" "") "null" }}
    AND  f.parent_id IS NULL
    {{- else }}
    AND  f.parent_id = {{ sqlVal "parentId" }}
    {{- end }}
  {{- end }}
  {{- if isSet "q" }}
    AND  f.name ILIKE {{ sqlVal "qPattern" }}
  {{- end }}
  {{- if isSet "category" }}
    {{- if eq (defaultOrValue "category" "all") "audios" }}
    AND  f.file_type ILIKE 'audio%'
    {{- else if eq (defaultOrValue "category" "all") "documents" }}
    AND  (f.file_type ILIKE 'application%' OR f.file_type ILIKE 'custom%')
    {{- else if eq (defaultOrValue "category" "all") "images" }}
    AND  f.file_type ILIKE 'image%'
    {{- else if eq (defaultOrValue "category" "all") "videos" }}
    AND  f.file_type ILIKE 'video%'
    {{- else if eq (defaultOrValue "category" "all") "websites" }}
    AND  f.file_type ILIKE 'text/html%'
    {{- end }}
  {{- end }}

  UNION ALL

  -- Personal documents (notes, not file-backed)
  SELECT
      id,
      file_id,
      id                             AS document_id,
      COALESCE(title, filename, 'Untitled') AS name,
      file_type,
      total_char_count               AS size,
      source                         AS url,
      created_at,
      updated_at,
      NULL::uuid                     AS chunk_task_id,
      NULL::uuid                     AS embedding_task_id,
      editor_data,
      content,
      slug,
      metadata,
      'document'                     AS source_type,
      NULL::int                      AS chunk_count,
      NULL::text                     AS chunking_status,
      NULL::jsonb                    AS chunking_error,
      NULL::text                     AS embedding_status,
      NULL::jsonb                    AS embedding_error,
      false                          AS finish_embedding
  FROM   documents
  {{- if isSet "workspaceId" }}
  WHERE  workspace_id = {{ sqlVal "workspaceId" }}
  {{- else if eq (defaultOrValue "workspaceScope" "") "all" }}
  WHERE  {{ workspaceScopeIn "workspace_id" }}
  {{- else }}
  WHERE  user_id = {{ sqlVal "userId" }} AND workspace_id IS NULL
  {{- end }}
    AND  source_type != 'file'
    AND  knowledge_base_id IS NULL
    -- filterKnowledgeItems: hide folders from inbox
    AND  NOT (source_type = 'document' AND file_type = 'custom/folder')
  {{- if isSet "parentId" }}
    {{- if eq (defaultOrValue "parentId" "") "null" }}
    AND  parent_id IS NULL
    {{- else }}
    AND  parent_id = {{ sqlVal "parentId" }}
    {{- end }}
  {{- end }}
  {{- if isSet "q" }}
    AND  (title ILIKE {{ sqlVal "qPattern" }} OR filename ILIKE {{ sqlVal "qPattern" }})
  {{- end }}
  {{- if isSet "category" }}
    {{- if eq (defaultOrValue "category" "all") "audios" }}
    AND  false
    {{- else if eq (defaultOrValue "category" "all") "documents" }}
    AND  (file_type ILIKE 'application%' OR (file_type ILIKE 'custom%' AND file_type != 'custom/document'))
    {{- else if eq (defaultOrValue "category" "all") "images" }}
    AND  false
    {{- else if eq (defaultOrValue "category" "all") "videos" }}
    AND  false
    {{- else if eq (defaultOrValue "category" "all") "websites" }}
    AND  false
    {{- end }}
  {{- end }}
) inbox_combined
{{- end }}

) all_items
ORDER BY
{{- if and (isSet "sorter") (isSet "sortType") }}
  {{- if eq (defaultOrValue "sorter" "") "createdAt" }}
    {{- if eq (defaultOrValue "sortType" "desc") "asc" }} created_at ASC
    {{- else }} created_at DESC {{- end }}
  {{- else if eq (defaultOrValue "sorter" "") "size" }}
    {{- if eq (defaultOrValue "sortType" "desc") "asc" }} size ASC
    {{- else }} size DESC {{- end }}
  {{- else if eq (defaultOrValue "sorter" "") "name" }}
    {{- if eq (defaultOrValue "sortType" "desc") "asc" }} name ASC
    {{- else }} name DESC {{- end }}
  {{- else if eq (defaultOrValue "sorter" "") "updatedAt" }}
    {{- if eq (defaultOrValue "sortType" "desc") "asc" }} updated_at ASC
    {{- else }} updated_at DESC {{- end }}
  {{- else }} created_at DESC
  {{- end }}
{{- else }} created_at DESC
{{- end }}
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "50") }};
