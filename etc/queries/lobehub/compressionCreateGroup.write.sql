-- compressionCreateGroup
-- Replaces: apps/server/src/services/message: createCompressionGroup
--           packages/database/src/repositories/compression: createCompressionGroup
--
-- Creates a compression group and marks messages as compressed.
-- Returns the created group ID.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   topicId    (text, required)
--   messageIds (text, required — comma-separated message IDs)
--   metadata   (text, optional — JSON string for metadata, default '{"originalMessageCount":0}')
WITH group_insert AS (
    INSERT INTO message_groups (
        id, topic_id, user_id, workspace_id, type, content, description
    )
    SELECT
        'mg_' || gen_random_uuid()::text,
        {{ sqlVal "topicId" }},
        {{ sqlVal "userId" }},
        {{ defaultOrNull "workspaceId" }},
        'compression',
        '...',
        COALESCE({{ defaultOrNull "metadata" }}, '{"originalMessageCount":0}')
    RETURNING id
),
mark_messages AS (
    UPDATE messages
    SET message_group_id = (SELECT id FROM group_insert)
    WHERE user_id = {{ sqlVal "userId" }}
      AND id = ANY(STRING_TO_ARRAY({{ sqlVal "messageIds" }}, ','))
)
SELECT id FROM group_insert;
