WITH ranked_voicebox AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            ORDER BY updated_at DESC NULLS LAST, created_at DESC NULLS LAST, id DESC
        ) AS rn
    FROM channel_instances
    WHERE channel_type = 'voicebox' AND enabled = true
)
UPDATE channel_instances
SET enabled = false, updated_at = NOW()
WHERE id IN (
    SELECT id
    FROM ranked_voicebox
    WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_channel_instances_voicebox_single_enabled
    ON channel_instances ((1))
    WHERE channel_type = 'voicebox' AND enabled = true;
