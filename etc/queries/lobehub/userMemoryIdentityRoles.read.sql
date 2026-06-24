-- userMemoryIdentityRoles
-- Replaces: apps/server router userMemories.queryIdentityRoles
--
-- Returns distinct role + relationship pairs from a user's identities.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   page (integer, optional, default 1)
--   size (integer, optional, default 50)
SELECT DISTINCT
    i.role,
    i.relationship
FROM   user_memories_identities i
JOIN   user_memories m ON m.id = i.user_memory_id
WHERE  m.user_id = {{ sqlVal "userId" }}
  AND  m.status = 'active'
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "50") }}