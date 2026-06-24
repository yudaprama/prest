-- documentFolderBreadcrumb
-- Replaces: apps/server router document.getFolderBreadcrumb
--
-- Walks the documents table parent_id chain to build the folder breadcrumb
-- path for a given document slug. Returns the ancestor chain as a JSON array.
--
-- Auth scope: userId (auto-injected from Kratos identity)
--
-- Query params:
--   slug (string, required)
WITH RECURSIVE ancestors AS (
    SELECT id, title, slug, parent_id, 0 AS depth
    FROM   documents
    WHERE  slug = {{ sqlVal "slug" }}
      AND  user_id = {{ sqlVal "userId" }}
    UNION ALL
    SELECT d.id, d.title, d.slug, d.parent_id, a.depth + 1
    FROM   documents d
    JOIN   ancestors a ON d.id = a.parent_id
    WHERE  d.user_id = {{ sqlVal "userId" }}
)
SELECT
    json_agg(
        json_build_object('id', id, 'title', title, 'slug', slug)
        ORDER BY depth DESC
    ) AS breadcrumb
FROM   ancestors;
