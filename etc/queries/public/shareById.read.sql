-- shareById (PUBLIC — anonymous, served via Oathkeeper `_QUERIES/public/*`)
--
-- Reads a shared conversation snapshot by its share id. No auth / no user
-- scoping: shares are public by design; revoked shares (revoked_at set) are
-- hidden. `content` is the UIMessage[] JSON captured at share time.
--
-- Query params:
--   id  (string, required) — the share id
SELECT id, title, content, created_at
FROM shares
WHERE id = {{ sqlVal "id" }}
  AND revoked_at IS NULL;
