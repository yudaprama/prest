-- compressionFinalize
-- Replaces: apps/server/src/services/message: finalizeCompression
--           packages/database/src/repositories/compression: updateCompressionContent
--
-- Updates compression group with actual summary content.
-- Merges metadata if provided.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   messageGroupId (text, required)
--   content        (text, required — the generated summary)
--   metadata       (text, optional — JSON string to merge into description)
UPDATE message_groups
SET
    content = {{ sqlVal "content" }},
    description = CASE
        WHEN {{ defaultOrNull "metadata" }} IS NOT NULL THEN
            COALESCE(description, '{}')::jsonb || {{ sqlVal "metadata" }}::jsonb
        ELSE
            description
    END,
    updated_at = now()
WHERE id = {{ sqlVal "messageGroupId" }}
  AND user_id = {{ sqlVal "userId" }}
RETURNING id, topic_id AS "topicId";
