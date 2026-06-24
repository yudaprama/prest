-- userMemoryDetail
-- Replaces: apps/server router userMemories.getMemoryDetail
--
-- Fetches a single memory by id, layer-joined to its child row, within
-- the caller's user scope. Returns null/zero rows if the memory belongs
-- to another user.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   id    (string, required) — memory id
--   layer (string, required) — one of: identity, activity, context, experience, preference
SELECT
    m.*,
{{- if eq .layer "identity" }}
    i.id            AS identity_id,
    i.description,
    i.relationship,
    i.role,
    i.type          AS identity_type,
    i.episodic_date,
    i.metadata      AS identity_metadata
FROM   user_memories m
JOIN   user_memories_identities i ON i.user_memory_id = m.id
{{- else if eq .layer "activity" }}
    a.id            AS activity_id,
    a.type          AS activity_type,
    a.status        AS activity_status,
    a.timezone,
    a.starts_at,
    a.ends_at,
    a.associated_objects,
    a.associated_subjects,
    a.associated_locations,
    a.notes,
    a.narrative,
    a.feedback,
    a.metadata      AS activity_metadata
FROM   user_memories m
JOIN   user_memories_activities a ON a.user_memory_id = m.id
{{- else if eq .layer "context" }}
    c.id            AS context_id,
    c.user_memory_ids,
    c.title         AS context_title,
    c.description,
    c.type          AS context_type,
    c.current_status,
    c.score_impact,
    c.score_urgency,
    c.metadata      AS context_metadata
FROM   user_memories m
JOIN   user_memories_contexts c ON c.user_memory_id = m.id
{{- else if eq .layer "experience" }}
    e.id            AS experience_id,
    e.type          AS experience_type,
    e.situation,
    e.reasoning,
    e.possible_outcome,
    e.action,
    e.key_learning,
    e.score_confidence,
    e.metadata      AS experience_metadata
FROM   user_memories m
JOIN   user_memories_experiences e ON e.user_memory_id = m.id
{{- else if eq .layer "preference" }}
    p.id            AS preference_id,
    p.type          AS preference_type,
    p.conclusion_directives,
    p.suggestions,
    p.score_priority,
    p.metadata      AS preference_metadata
FROM   user_memories m
JOIN   user_memories_preferences p ON p.user_memory_id = m.id
{{- else }}
    NULL::text      AS child_id
FROM   user_memories m
{{- end }}
WHERE  m.user_id = {{ sqlVal "userId" }}
   AND m.id = {{ sqlVal "id" }}
LIMIT  1;