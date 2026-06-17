-- messengerInstallationsByUser
-- Replaces: routers/lambda/messenger.ts: getUserInstallations
--
-- Lists all messenger platform installations tied to the current user.
-- messenger_installations uses installed_by_user_id, not user_id, so this
-- template is the safe scoped access path. Returns only non-revoked installs.
--
-- Auth scope:   userId (auto-injected from Kratos identity; maps to
--               installed_by_user_id, not the canonical user_id column)
--
-- Query params:
--   platform   (string, optional) — filter to one platform ('slack', etc.)
--   page       (int,    default 1)
--   size       (int,    default 20)
--
-- Returns: array of messenger_installations with metadata (no credentials —
--          those stay in the BFF's key vault decryption).

SELECT
    id,
    platform,
    tenant_id          AS "tenantId",
    application_id     AS "applicationId",
    account_id         AS "accountId",
    metadata,
    token_expires_at   AS "tokenExpiresAt",
    installed_by_user_id AS "installedByUserId",
    created_at         AS "createdAt",
    updated_at         AS "updatedAt"
FROM   messenger_installations
WHERE  installed_by_user_id = {{ sqlVal "userId" }}
  AND  revoked_at IS NULL
{{- if isSet "platform" }}
  AND  platform = {{ sqlVal "platform" }}
{{- end }}
ORDER  BY created_at DESC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "20") }};
