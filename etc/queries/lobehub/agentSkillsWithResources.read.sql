-- agentSkillsWithResources
-- Replaces: routers/lambda/agentSkills.ts: list / listResources
--           models/agentSkill.ts:  listBySource
--
-- The `resources` column is a Record<VirtualPath, { hash, size, mime?, ... }>
-- in JSONB form. Unfold it with jsonb_each, then lateral-join into global_files
-- by hashId to surface the resolved URL and storage metadata for each
-- resource entry.
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   source          (string, optional) — 'builtin' | 'market' | 'user'
--   includeContent  (bool,   default false) — include the skill body text
--                                              (can be large; off by default)
--   page            (int,    default 1)
--   size            (int,    default 20)
--
-- Returns: array of skills with their resource entries inlined as
--          `resources[]` (virtualPath, hash, size, mime, resolvedUrl, etc.).
SELECT
    s.id,
    s.name,
    s.description,
    s.identifier,
    s.source,
    s.manifest,
    {{- if eq (defaultOrValue "includeContent" "false") "true" }}
    s.content,
    s.editor_data,
    {{- end }}
    s.zip_file_hash,
    s.user_id,
    s.workspace_id,
    s.created_at,
    s.updated_at,
    COALESCE(
        (SELECT json_agg(
                  jsonb_build_object(
                      'virtual_path',  kv.key,
                      'hash',          (kv.value->>'hash'),
                      'size',          (kv.value->>'size')::bigint,
                      'mime',          (kv.value->>'mime'),
                      'resolved_url',  gf.url,
                      'file_type',     gf.file_type,
                      'global_metadata', gf.metadata
                  )
                  ORDER BY kv.key)
         FROM   jsonb_each(s.resources) kv
         LEFT JOIN global_files gf ON gf.hash_id = (kv.value->>'hash')),
        '[]'::json
    ) AS resources
FROM   agent_skills s
{{- if isSet "workspaceId" }}
WHERE  s.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  s.user_id = {{ sqlVal "userId" }} AND s.workspace_id IS NULL
{{- end }}
{{- if isSet "source" }}
  AND  s.source = {{ sqlVal "source" }}
{{- end }}
ORDER  BY s.updated_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
