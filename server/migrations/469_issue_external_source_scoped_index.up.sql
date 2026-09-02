CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_issue_external_source_unique
    ON issue (
        workspace_id,
        (metadata->>'source_system'),
        (COALESCE(metadata->>'source_scope', '')),
        (metadata->>'source_id')
    )
    WHERE metadata ? 'source_system'
      AND metadata ? 'source_id'
      AND COALESCE(metadata->>'source_system', '') <> ''
      AND COALESCE(metadata->>'source_id', '') <> '';
