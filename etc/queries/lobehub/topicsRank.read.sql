-- topicsRank
-- Replaces: routers/lambda/topic.ts: rankTopics (TopicModel.rank)
--
-- Auth scope:   userId  (auto-injected from Kratos identity)
--
-- Query params:
--   limit  (int, default 10) — max ranked topics to return
--
-- Returns: topics ranked by message count (descending), excluding topics with
--          no messages. Shape matches TopicRankItem { agentId, count, id, title }.
--
-- Notes:
--   * GROUP BY t.id is sufficient — `topics.id` is the primary key, so Postgres
--     lets us select t.title / t.agent_id without aggregating them (functional
--     dependency). Mirrors the Drizzle `TopicModel.rank` query.
SELECT
    t.id,
    t.title,
    t.agent_id,
    COUNT(m.id)::int AS count
FROM   topics t
LEFT   JOIN messages m ON m.topic_id = t.id
WHERE  t.user_id = {{ sqlVal "userId" }}
GROUP  BY t.id
HAVING COUNT(m.id) > 0
ORDER  BY COUNT(m.id) DESC
{{ limitOffset "1" (defaultOrValue "limit" "10") }};
