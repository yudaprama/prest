-- documentsWithHistory
-- Replaces: routers/lambda/document.ts: listDocumentHistory
--           services/document/history.ts: listDocumentHistory
--
-- Cursor-paginated history for a single document, joining document_histories
-- with the current document head. Supports historySince (free-tier cutoff),
-- cursor pagination via beforeSavedAt/beforeId, and an optional includeCurrent
-- flag to include the document's current editorData as the first entry.
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   documentId     (string, required) — document to list history for
--   beforeSavedAt  (string, optional) — cursor: return entries older than this ISO timestamp
--   beforeId       (string, optional) — cursor: return entries older than this ID (tiebreaker)
--   limit          (int,    default 20)
--   includeCurrent (string, optional) — if 'true', prepend the current doc head
--   historySince   (string, optional) — ISO timestamp: only entries newer than this (free tier cutoff)
--
-- Returns: array of { id, documentId, editorData, saveSource, savedAt }
--          ordered by savedAt DESC, id DESC.

WITH history AS (
    SELECT
        dh.id,
        dh.document_id        AS "documentId",
        dh.editor_data        AS "editorData",
        dh.save_source        AS "saveSource",
        dh.saved_at           AS "savedAt",
        dh.user_id,
        dh.workspace_id
    FROM   document_histories dh
    WHERE  dh.document_id = {{ sqlVal "documentId" }}
{{- if isSet "workspaceId" }}
      AND dh.workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
      AND dh.user_id = {{ sqlVal "userId" }} AND dh.workspace_id IS NULL
{{- end }}
{{- if isSet "beforeSavedAt" }}
      AND (dh.saved_at, dh.id) < ({{ sqlVal "beforeSavedAt" }}::timestamptz, {{ sqlVal "beforeId" }})
{{- end }}
{{- if isSet "historySince" }}
      AND dh.saved_at >= {{ sqlVal "historySince" }}::timestamptz
{{- end }}
)
SELECT * FROM history
ORDER BY "savedAt" DESC, id DESC
{{ limitOffset "1" (defaultOrValue "limit" "20") }};
