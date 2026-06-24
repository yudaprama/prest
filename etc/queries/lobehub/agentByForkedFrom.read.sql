-- agentByForkedFrom
-- Replaces: apps/server router agent.getAgentByForkedFromIdentifier
--
-- Looks up an agent by the forkedFromIdentifier stored in its params JSONB.
-- Returns the most recently updated match.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   forkedFromIdentifier (string, required) — the source agent's market identifier
SELECT a.id
FROM agents a
WHERE a.user_id = {{ sqlVal "userId" }}
  AND a.workspace_id IS NULL
  AND a.params->>'forkedFromIdentifier' = {{ sqlVal "forkedFromIdentifier" }}
ORDER BY a.updated_at DESC
LIMIT 1;
