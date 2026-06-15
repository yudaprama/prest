-- verifyResultsWithRubric
-- Replaces: routers/lambda/verify.ts: listResults
--           models/verifyCheckResult.ts: listByOperation
--
-- The check_result row already carries a flattened snapshot of the originating
-- criterion (title, required, verifier_type, verifier_config_hash), so no join
-- to verify_criteria is needed for the read path. The criterion source link
-- lives inside agent_operations.verify_plan (JSON), not as a FK column.
--
-- Auth scope:   userId      (auto-injected from Kratos identity)
--               workspaceId (optional query param — if set, scope to workspace;
--                            else personal scope with workspace_id IS NULL)
--
-- Query params:
--   operationId (string, required)
--   page        (int,    default 1)
--   size        (int,    default 50)
--
-- Returns: array of verify_check_results for one agent operation, ordered by
--          the check item display index.
SELECT
    id,
    operation_id,
    check_item_id,
    check_item_title,
    check_item_index,
    required,
    verifier_type,
    verifier_config_hash,
    verifier_operation_id,
    verifier_tracing_id,
    status,
    verdict,
    confidence,
    toulmin,
    suggestion,
    user_decision,
    is_false_positive,
    is_false_negative,
    repair_operation_id,
    started_at,
    completed_at,
    created_at
FROM   verify_check_results
{{- if isSet "workspaceId" }}
WHERE  workspace_id = {{ sqlVal "workspaceId" }}
{{- else }}
WHERE  user_id = {{ sqlVal "userId" }} AND workspace_id IS NULL
{{- end }}
  AND  operation_id = {{ sqlVal "operationId" }}
ORDER  BY check_item_index ASC NULLS LAST, created_at ASC
{{ limitOffset (defaultOrValue "page" "1") (defaultOrValue "size" "50") }};
