-- userMemoryPersonaCurrent
-- Replaces: apps/server router userMemory.getPersona
--
-- Returns the latest persona document for a user, by profile.
-- The UI uses this for the persona landing card.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   profile (string, optional, default 'default') — persona profile name
SELECT
    id,
    persona,
    tagline,
    version,
    memory_ids,
    source_ids,
    metadata,
    captured_at,
    created_at,
    updated_at
FROM   user_memory_persona_documents
WHERE  user_id = {{ sqlVal "userId" }}
  AND  profile = {{ defaultOrValue "profile" "'default'" }}
ORDER  BY version DESC
LIMIT  1;