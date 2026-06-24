-- compressionUpdateMetadata
-- Replaces: apps/server/src/services/message: updateMessageGroupMetadata
--           packages/database/src/repositories/compression: updateMetadata
--
-- Merges new metadata into the message group's metadata JSONB column.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   messageGroupId (text, required)
--   metadata       (text, required — JSON string to merge)
UPDATE message_groups
SET
    metadata = COALESCE(metadata, '{}'::jsonb) || {{ sqlVal "metadata" }}::jsonb,
    updated_at = now()
WHERE id = {{ sqlVal "messageGroupId" }}
  AND user_id = {{ sqlVal "userId" }}
RETURNING id, topic_id AS "topicId";
