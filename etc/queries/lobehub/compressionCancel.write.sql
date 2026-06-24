-- compressionCancel
-- Replaces: apps/server/src/services/message: cancelCompression
--           packages/database/src/repositories/compression: deleteCompressionGroup
--
-- Cancels compression: unmarks all messages from the group, then deletes the group.
-- Returns the topic_id so the FE can re-query messages.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   messageGroupId (text, required)
WITH unmark AS (
    UPDATE messages
    SET message_group_id = NULL
    WHERE user_id = {{ sqlVal "userId" }}
      AND message_group_id = {{ sqlVal "messageGroupId" }}
),
delete_group AS (
    DELETE FROM message_groups
    WHERE id = {{ sqlVal "messageGroupId" }}
      AND user_id = {{ sqlVal "userId" }}
    RETURNING topic_id
)
SELECT topic_id AS "topicId" FROM delete_group;
