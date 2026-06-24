-- userMemoryTaxonomyOptions
-- Replaces: apps/server router userMemories.queryTaxonomyOptions
--
-- Returns aggregated taxonomy options for memory filtering in a single query.
-- Each row has a 'taxonomy' discriminator column so the FE can split results
-- into the 7 arrays of QueryTaxonomyOptionsResult.
--
-- Auth scope: userId (auto-injected from Kratos identity)

SELECT 'categories' AS taxonomy, memory_category AS value, COUNT(*)::int AS count
FROM user_memories
WHERE user_id = {{ sqlVal "userId" }}
  AND memory_category IS NOT NULL AND memory_category != ''
GROUP BY memory_category
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "50" }}

UNION ALL

SELECT 'types' AS taxonomy, memory_type AS value, COUNT(*)::int AS count
FROM user_memories
WHERE user_id = {{ sqlVal "userId" }}
  AND memory_type IS NOT NULL AND memory_type != ''
GROUP BY memory_type
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "50" }}

UNION ALL

SELECT 'statuses' AS taxonomy, status AS value, COUNT(*)::int AS count
FROM user_memories
WHERE user_id = {{ sqlVal "userId" }}
  AND status IS NOT NULL AND status != ''
GROUP BY status
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "50" }}

UNION ALL

SELECT 'tags' AS taxonomy, tag AS value, COUNT(*)::int AS count
FROM user_memories, unnest(tags) AS tag
WHERE user_id = {{ sqlVal "userId" }}
  AND tags IS NOT NULL
GROUP BY tag
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "50" }}

UNION ALL

SELECT 'labels' AS taxonomy, memory_layer AS value, COUNT(*)::int AS count
FROM user_memories
WHERE user_id = {{ sqlVal "userId" }}
  AND memory_layer IS NOT NULL AND memory_layer != ''
GROUP BY memory_layer
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "50" }}

UNION ALL

SELECT 'relationships' AS taxonomy, relationship AS value, COUNT(*)::int AS count
FROM user_memories_identities
WHERE user_id = {{ sqlVal "userId" }}
  AND relationship IS NOT NULL AND relationship != ''
GROUP BY relationship
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "50" }}

UNION ALL

SELECT 'roles' AS taxonomy, role AS value, COUNT(*)::int AS count
FROM user_memories_identities
WHERE user_id = {{ sqlVal "userId" }}
  AND role IS NOT NULL AND role != ''
GROUP BY role
ORDER BY count DESC
LIMIT {{ defaultOrValue "limit" "50" }};
