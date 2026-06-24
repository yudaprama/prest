-- resolveKnowledgeItemIds
-- Replaces: apps/server router file.resolveKnowledgeItemIds
--
-- Resolves file IDs to knowledge item IDs via the knowledge_items table.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   fileIds (string[], required — passed as comma-separated or JSON array)
SELECT
    ki.id          AS knowledge_item_id,
    ki.file_id     AS file_id
FROM   knowledge_items ki
WHERE  ki.user_id = {{ sqlVal "userId" }}
  AND  ki.file_id = ANY(STRING_TO_ARRAY({{ sqlVal "fileIds" }}, ','));
